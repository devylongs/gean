package aggregation

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

type aggregationGroup struct {
	dataRoot   [32]byte
	targetSlot uint64
}

// orderedGroups lists the snapshot's aggregation work frontier-first: by
// ascending target slot. Finalization advances only when the checkpoint
// immediately after the current source is justified (leanSpec
// process_attestations finalizes a source when no justifiable slot sits between
// it and its justified target). So when a backlog does not all fit the session
// budget, spending it on the lowest unjustified targets keeps finalization
// moving; ordering newest-first would advance the head while the finalization
// frontier starves — the shape of the observed stall (head advancing, finality
// lagging). Only the group order changes; every aggregate produced is spec-valid.
// groupSkips counts the groups a session dropped, by reason. Without it a
// session that drops every group is reported as produced=0, which reads exactly
// like having nothing to aggregate — the ambiguity that hid an aggregator
// producing nothing for 355 consecutive slots on devnet-5.
type groupSkips map[string]int

func (g groupSkips) add(reason string) {
	if g != nil {
		g[reason]++
	}
}

func (g groupSkips) total() int {
	n := 0
	for _, v := range g {
		n += v
	}
	return n
}

// summary renders the non-zero reasons in a stable order for logging.
func (g groupSkips) summary() string {
	if g.total() == 0 {
		return ""
	}
	reasons := make([]string, 0, len(g))
	for reason := range g {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	var b strings.Builder
	for _, reason := range reasons {
		if g[reason] == 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%s=%d", reason, g[reason])
	}
	return b.String()
}

func orderedGroups(snap *Snapshot, skips groupSkips) []aggregationGroup {
	dataRoots := make(map[[32]byte]bool)
	for dr := range snap.attSigs {
		dataRoots[dr] = true
	}
	for dr := range snap.newEntries {
		dataRoots[dr] = true
	}

	groups := make([]aggregationGroup, 0, len(dataRoots))
	for dr := range dataRoots {
		attData := attestationDataForRoot(snap, dr)
		if attData == nil {
			continue
		}
		// The target checkpoint drives finalization; fall back to the attestation
		// slot only for a malformed entry with no target (validation normally
		// guarantees one).
		targetSlot := attData.Slot
		if attData.Target != nil {
			targetSlot = attData.Target.Slot
		}
		// Skip targets already justified in the head state. process_attestations
		// ignores a vote once its target is justified, so proving it spends the
		// session budget on an aggregate that can no longer advance finality —
		// budget that a still-unjustified target needs. Only a definite "yes"
		// skips: an out-of-range target (beyond the tracked bitfield, i.e. a fresh
		// slot) returns an error and is kept.
		if snap.headState != nil && snap.headState.LatestFinalized != nil {
			justified, err := statetransition.IsSlotJustified(snap.headState, snap.headState.LatestFinalized.Slot, targetSlot)
			if err == nil && justified {
				skips.add(metrics.AggGroupSkipTargetJustified)
				continue
			}
		}
		groups = append(groups, aggregationGroup{dataRoot: dr, targetSlot: targetSlot})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].targetSlot != groups[j].targetSlot {
			return groups[i].targetSlot < groups[j].targetSlot
		}
		return bytes.Compare(groups[i].dataRoot[:], groups[j].dataRoot[:]) < 0
	})
	return groups
}

// seedPerUnitSeconds is a conservative starting cost for one aggregation unit
// (one raw signature or one child proof). It only governs the first pass, before
// any real timing is observed: a high seed makes that pass trim rather than risk
// spending the whole session budget on a single group. The estimate then
// converges to the real prover cost of whatever hardware runs the node.
const seedPerUnitSeconds = 0.1

// seedPerGroupSeconds is used until a successful group supplies a wall-time sample.
const seedPerGroupSeconds = 0.3

// unitCostEstimator tracks observed aggregation-proving time so each pass can be
// sized to the remaining session budget. The single-threaded worker holds one
// across dispatches. No fixed cap would hold across machines and validator-set
// sizes, so it self-calibrates instead.
type unitCostEstimator struct {
	perUnitSeconds  float64
	perGroupSeconds float64
}

func newUnitCostEstimator() *unitCostEstimator {
	// perGroupSeconds is left zero so the first observed group adopts its real
	// cost directly (nextGroupDuration falls back to the seed until then). This
	// makes the bound react within one group when proofs suddenly cost seconds,
	// rather than easing toward it over many sessions.
	return &unitCostEstimator{perUnitSeconds: seedPerUnitSeconds}
}

// nextGroupDuration estimates preparation plus proving time for admission after
// the first attempt. It cannot bound an in-flight proof's actual duration.
func (e *unitCostEstimator) nextGroupDuration() time.Duration {
	secs := seedPerGroupSeconds
	if e != nil && e.perGroupSeconds > 0 {
		secs = e.perGroupSeconds
	}
	return time.Duration(secs * float64(time.Second))
}

// observeGroup folds a completed group's realized wall time into the estimate
// with an exponential moving average, so the bound tracks the cost growth that
// comes with a larger state and validator set.
func (e *unitCostEstimator) observeGroup(duration time.Duration) {
	if e == nil || duration <= 0 {
		return
	}
	const alpha = 0.3
	if e.perGroupSeconds <= 0 {
		e.perGroupSeconds = duration.Seconds()
		return
	}
	e.perGroupSeconds = alpha*duration.Seconds() + (1-alpha)*e.perGroupSeconds
}

// maxUnitsWithin reports how many units fit in the remaining budget at the current
// estimate. It never returns below the spec minimum of two, so a group can always
// still produce a valid aggregate.
func (e *unitCostEstimator) maxUnitsWithin(budget time.Duration) int {
	if e == nil || e.perUnitSeconds <= 0 || budget <= 0 {
		return 2
	}
	fit := int(budget.Seconds() / e.perUnitSeconds)
	if fit < 2 {
		return 2
	}
	return fit
}

// observe folds a completed aggregation's realized per-unit cost into the estimate
// with an exponential moving average, damping single-pass noise. Under Shadow the
// duration includes the modeled prover sleep, so the estimate calibrates to the
// simulated cost just as it would to real hardware.
func (e *unitCostEstimator) observe(duration time.Duration, units int) {
	if e == nil || units <= 0 || duration <= 0 {
		return
	}
	const alpha = 0.3
	sample := duration.Seconds() / float64(units)
	e.perUnitSeconds = alpha*sample + (1-alpha)*e.perUnitSeconds
}

func aggregateFromSnapshot(snap *Snapshot, cache *xmss.PubKeyCache, deadline time.Time, shadowRates shadow.Rates, estimator *unitCostEstimator) ([]*types.SignedAggregatedAttestation, []store.PayloadKV, []store.AttestationDeleteKey, bool, groupSkips) {
	return aggregateFromSnapshotWithProver(snap, cache, deadline, shadowRates, estimator, xmss.AggregateWithChildren)
}

func aggregateFromSnapshotWithProver(snap *Snapshot, cache *xmss.PubKeyCache, deadline time.Time, shadowRates shadow.Rates, estimator *unitCostEstimator, prove func([]xmss.CPubKey, []xmss.CSig, []xmss.ChildProof, [32]byte, uint32) ([]byte, error)) ([]*types.SignedAggregatedAttestation, []store.PayloadKV, []store.AttestationDeleteKey, bool, groupSkips) {
	skips := groupSkips{}
	if snap == nil || cache == nil || snap.headState == nil {
		return nil, nil, nil, false, skips
	}
	if estimator == nil {
		estimator = newUnitCostEstimator()
	}

	var newAggregates []*types.SignedAggregatedAttestation
	var payloadEntries []store.PayloadKV
	var keysToDelete []store.AttestationDeleteKey
	truncated := false
	attempted := false

	for _, group := range orderedGroups(snap, skips) {
		// An over-budget observation must not prevent every future attempt:
		// without a successful proof the estimator cannot recalibrate. Allow
		// one attempt while time remains; subsequent attempts use the estimate.
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 || (attempted && remaining < estimator.nextGroupDuration()) {
				truncated = true
				skips.add(metrics.AggGroupSkipBudget)
				break
			}
		}
		dataRoot := group.dataRoot
		groupStart := time.Now()
		provedBefore := len(newAggregates)
		func() {
			childProofsBuf := getChildProofsBuf()
			defer putChildProofsBuf(childProofsBuf)
			rawPubkeysBuf := getRawPubkeysBuf()
			defer putRawPubkeysBuf(rawPubkeysBuf)
			rawSigsBuf := getRawSigsBuf()
			defer putRawSigsBuf(rawSigsBuf)
			rawIDsBuf := getRawIDsBuf()
			defer putRawIDsBuf(rawIDsBuf)

			prepStart := time.Now()
			gossipEntry := snap.attSigs[dataRoot]
			newEntry := snap.newEntries[dataRoot]
			knownEntry := snap.knownEntries[dataRoot]

			// Non-nil: orderedGroups already dropped roots without data.
			attData := attestationDataForRoot(snap, dataRoot)

			// Signers are resolved against the head state's registry. The
			// validator set is written once at genesis and never by the state
			// transition, so every state on the chain carries the same registry
			// and the head's is equivalent to the vote's target. Reading the
			// target's own state instead cost a store lookup per data root and
			// silently dropped the group whenever that state was absent — which
			// on devnet-5 was every group, every slot.
			registry := snap.headState.Validators

			// Bound this pass to what fits the remaining session budget at the
			// current per-unit estimate. Child proofs go in first (most coverage
			// per unit); fresh raw signatures take whatever budget is left and the
			// rest are deferred to the next pass. A smaller aggregate over the
			// included participants is still spec-valid.
			remaining := estimator.maxUnitsWithin(time.Until(deadline))

			covered := make(map[uint64]bool)
			selectChildProofs(newEntry, snap.headState, childProofsBuf, covered, cache, &remaining)
			selectChildProofs(knownEntry, snap.headState, childProofsBuf, covered, cache, &remaining)

			if gossipEntry != nil && len(gossipEntry.Signatures) > 0 {
				sortedSigs := make([]store.AttestationSignatureEntry, len(gossipEntry.Signatures))
				copy(sortedSigs, gossipEntry.Signatures)
				sort.Slice(sortedSigs, func(i, j int) bool {
					return sortedSigs[i].ValidatorID < sortedSigs[j].ValidatorID
				})

				for _, sigEntry := range sortedSigs {
					if remaining <= 0 {
						break
					}
					if covered[sigEntry.ValidatorID] {
						continue
					}
					if sigEntry.ValidatorID >= uint64(len(registry)) {
						continue
					}

					// Parse a handle the worker owns and frees at the end of this
					// group. The snapshot carries only bytes, never a handle shared
					// with the live map, so a concurrent prune cannot free it underneath
					// the prover.
					sigHandle, err := xmss.ParseSignature(sigEntry.Signature[:])
					if err != nil {
						continue
					}
					defer xmss.FreeSignature(sigHandle)

					pk, err := cache.Get(registry[sigEntry.ValidatorID].AttestationPubkey)
					if err != nil {
						continue
					}

					*rawPubkeysBuf = append(*rawPubkeysBuf, pk)
					*rawSigsBuf = append(*rawSigsBuf, sigHandle)
					*rawIDsBuf = append(*rawIDsBuf, sigEntry.ValidatorID)
					remaining--
				}
			}

			if len(*rawIDsBuf)+len(*childProofsBuf) < 2 {
				skips.add(metrics.AggGroupSkipTooFewSigners)
				return
			}

			dataRootHash, slot, err := aggregationMessage(attData)
			if err != nil {
				logger.Error(logger.Signature, "aggregate: prepare message failed slot=%d: %v", attData.Slot, err)
				skips.add(metrics.AggGroupSkipError)
				return
			}

			metrics.ObserveAggregationPrepTime(time.Since(prepStart).Seconds())

			// Preparation can consume the remaining time. Once started, proving
			// cannot be interrupted by this deadline.
			if !deadline.IsZero() && time.Until(deadline) <= 0 {
				truncated = true
				skips.add(metrics.AggGroupSkipBudget)
				return
			}
			attempted = true
			aggStart := time.Now()
			proofBytes, err := prove(*rawPubkeysBuf, *rawSigsBuf, *childProofsBuf, dataRootHash, slot)
			// Charge virtual time for the proving cost Shadow would otherwise not
			// account; the capacity-1 dispatch channel then drops the next slot's
			// work if proving can't keep up, exactly as on real hardware.
			shadowRates.SleepAggregate(len(*rawIDsBuf) + len(*childProofsBuf))
			aggDuration := time.Since(aggStart)
			if err != nil {
				logger.Error(logger.Signature, "aggregate: failed slot=%d raw=%d children=%d duration=%v: %v",
					slot, len(*rawIDsBuf), len(*childProofsBuf), aggDuration, err)
				skips.add(metrics.AggGroupSkipError)
				return
			}
			estimator.observe(aggDuration, len(*rawIDsBuf)+len(*childProofsBuf))

			allIDs := make([]uint64, 0, len(*rawIDsBuf)+len(covered))
			allIDs = append(allIDs, (*rawIDsBuf)...)
			for vid := range covered {
				allIDs = append(allIDs, vid)
			}

			proof := &types.SingleMessageAggregate{
				Participants: types.BitlistFromIndices(allIDs),
				Proof:        proofBytes,
			}

			logger.Info(logger.Signature, "aggregate: slot=%d raw=%d children=%d total=%d proof=%d bytes duration=%v",
				slot, len(*rawIDsBuf), len(*childProofsBuf), len(allIDs), len(proofBytes), aggDuration)

			metrics.ObservePqSigAggBuildingTime(aggDuration.Seconds())
			metrics.ObserveCommitteeSignaturesAggregationTime(aggDuration.Seconds())
			metrics.IncPqSigAggregatedTotal()
			metrics.IncPqSigAttestationsInAggregated(len(allIDs))

			newAggregates = append(newAggregates, &types.SignedAggregatedAttestation{
				Data:  attData,
				Proof: proof,
			})

			payloadEntries = append(payloadEntries, store.PayloadKV{
				DataRoot: dataRoot,
				Data:     attData,
				Proof:    proof,
			})

			// Only retire gossip signatures whose vote made it into this proof.
			// Signatures the budget deferred stay in the store for the next pass;
			// retiring them here would drop those votes silently.
			if gossipEntry != nil {
				represented := make(map[uint64]bool, len(allIDs))
				for _, vid := range allIDs {
					represented[vid] = true
				}
				for _, sig := range gossipEntry.Signatures {
					if !represented[sig.ValidatorID] {
						continue
					}
					keysToDelete = append(keysToDelete, store.AttestationDeleteKey{
						ValidatorID: sig.ValidatorID,
						DataRoot:    dataRoot,
					})
				}
			}
		}()
		if truncated {
			break
		}
		// Only groups that actually proved inform the wall-time estimate; skipped
		// groups (too few signatures) return fast and would bias it low.
		if len(newAggregates) > provedBefore {
			estimator.observeGroup(time.Since(groupStart))
		}
	}

	return newAggregates, payloadEntries, keysToDelete, truncated, skips
}

func aggregationMessage(attData *types.AttestationData) ([32]byte, uint32, error) {
	if attData == nil {
		return [32]byte{}, 0, fmt.Errorf("attestation data is nil")
	}
	dataRoot, err := attData.HashTreeRoot()
	if err != nil {
		return [32]byte{}, 0, fmt.Errorf("attestation data root: %w", err)
	}
	slot := uint32(attData.Slot)
	if uint64(slot) != attData.Slot {
		return [32]byte{}, 0, fmt.Errorf("slot %d overflows uint32", attData.Slot)
	}
	return dataRoot, slot, nil
}
