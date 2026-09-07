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
	// currentSlot marks a vote cast in the slot being aggregated for, as opposed
	// to a backlog entry carried over from an earlier one.
	currentSlot bool
}

// orderedGroups lists the snapshot's aggregation work current-slot first, then
// frontier-first by ascending target slot.
//
// This slot's votes are the only ones with a deadline: they must be aggregated
// and gossiped in time to reach the next block, while a backlog entry loses
// nothing by waiting a slot. Ordering purely by target slot puts the oldest
// backlog ahead of them, so a session capped at two groups can spend both on
// stale work and let the current slot's own votes go unaggregated.
//
// Within each tier the frontier rule stands. Finalization advances only when
// the checkpoint
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
	g.addN(reason, 1)
}

// addN records n groups dropped for the same reason. A budget stop defers every
// remaining group, not just the one it examined, so counting one understates the
// backlog a short session leaves behind.
func (g groupSkips) addN(reason string, n int) {
	if g != nil && n > 0 {
		g[reason] += n
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
		groups = append(groups, aggregationGroup{
			dataRoot:    dr,
			targetSlot:  targetSlot,
			currentSlot: attData.Slot == snap.slot,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].currentSlot != groups[j].currentSlot {
			return groups[i].currentSlot
		}
		if groups[i].targetSlot != groups[j].targetSlot {
			return groups[i].targetSlot < groups[j].targetSlot
		}
		return bytes.Compare(groups[i].dataRoot[:], groups[j].dataRoot[:]) < 0
	})
	return groups
}

// seedPerChildSeconds is the starting cost for one child proof, used until a
// group carrying one supplies a real sample. Measured on a 16-core host, adding
// a child roughly tripled a group's proving time; the seed sits nearer the low
// end so the first pass is not paralysed before any timing is observed.
const seedPerChildSeconds = 1.5

// seedPerGroupSeconds is used until a successful group supplies a wall-time sample.
const seedPerGroupSeconds = 0.3

// MaxGroupsPerSession bounds how many groups one session hands to the prover.
// Without it a session costs whatever the backlog costs, which is how four
// aggregators covering four subnets saturated a 16-core host and left the node
// 129 slots behind. A count is a cruder bound than the wall-clock deadline, but
// it is the one that holds before any proof has started, so the gate token is
// never held for an unbounded stretch. ethlambda uses the same value.
const MaxGroupsPerSession = 2

// MaxGroupsWhenProposing applies in the slot before this node proposes. The
// proving gate gives a proposal priority, but priority only defers the next
// background acquire: a session already holding the token runs to completion,
// so the proposal waits for it. One group bounds that wait.
const MaxGroupsWhenProposing = 1

// unitCostEstimator tracks observed aggregation-proving time so each pass can be
// sized to the remaining session budget. The single-threaded worker holds one
// across dispatches. No fixed cap would hold across machines and validator-set
// sizes, so it self-calibrates instead.
// Proving cost is dominated by a fixed per-proof term rather than by how much
// the proof covers: measured on a 16-core host, a group of two raw signatures
// took 2.0-5.2s and produced ~146 KB of proof, and carrying more signatures
// barely moved either figure. Cost is modelled as perGroupSeconds plus
// children x perChildSeconds, with raw signatures free at the margin.
//
// The previous model divided a group's duration by its signature count and read
// the result as a per-signature price. That charged the whole fixed cost to
// whichever signatures happened to be in the group, concluded a signature costs
// seconds, and sized every later group at the two-signature floor — a
// self-confirming estimate that paid full price for half the coverage.
type unitCostEstimator struct {
	perChildSeconds float64
	perGroupSeconds float64
}

func newUnitCostEstimator() *unitCostEstimator {
	// perGroupSeconds is left zero so the first observed group adopts its real
	// cost directly (nextGroupDuration falls back to the seed until then). This
	// makes the bound react within one group when proofs suddenly cost seconds,
	// rather than easing toward it over many sessions.
	return &unitCostEstimator{perChildSeconds: seedPerChildSeconds}
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

// observeGroup folds a completed group's realized wall time into the estimates.
// A raw-only group prices the fixed per-proof cost directly. A group carrying
// children charges whatever the fixed cost does not explain to those children,
// which is well conditioned because raw-only groups are the common case once
// selection is raw-first.
func (e *unitCostEstimator) observeGroup(duration time.Duration, childCount int) {
	if e == nil || duration <= 0 {
		return
	}
	const alpha = 0.3

	if childCount == 0 {
		if e.perGroupSeconds <= 0 {
			// Adopt the first sample outright rather than easing toward it, so
			// the bound reacts within one group when proofs suddenly cost
			// seconds instead of converging over many sessions.
			e.perGroupSeconds = duration.Seconds()
			return
		}
		e.perGroupSeconds = alpha*duration.Seconds() + (1-alpha)*e.perGroupSeconds
		return
	}

	if e.perGroupSeconds <= 0 {
		// No fixed-cost baseline yet, so nothing can be attributed to children.
		return
	}
	residual := duration.Seconds() - e.perGroupSeconds
	if residual <= 0 {
		// The group came in under what a raw-only group costs, so it says
		// nothing about the children it carried.
		return
	}
	e.perChildSeconds = alpha*(residual/float64(childCount)) + (1-alpha)*e.perChildSeconds
}

// childDuration is the wall time one more child proof is expected to add.
func (e *unitCostEstimator) childDuration() time.Duration {
	secs := seedPerChildSeconds
	if e != nil && e.perChildSeconds > 0 {
		secs = e.perChildSeconds
	}
	return time.Duration(secs * float64(time.Second))
}

func aggregateFromSnapshot(snap *Snapshot, cache *xmss.PubKeyCache, deadline time.Time, maxGroups int, shadowRates shadow.Rates, estimator *unitCostEstimator) ([]*types.SignedAggregatedAttestation, []store.PayloadKV, []store.AttestationDeleteKey, bool, groupSkips) {
	return aggregateFromSnapshotWithProver(snap, cache, deadline, maxGroups, shadowRates, estimator, xmss.AggregateWithChildren)
}

func aggregateFromSnapshotWithProver(snap *Snapshot, cache *xmss.PubKeyCache, deadline time.Time, maxGroups int, shadowRates shadow.Rates, estimator *unitCostEstimator, prove func([]xmss.CPubKey, []xmss.CSig, []xmss.ChildProof, [32]byte, uint32) ([]byte, error)) ([]*types.SignedAggregatedAttestation, []store.PayloadKV, []store.AttestationDeleteKey, bool, groupSkips) {
	skips := groupSkips{}
	if snap == nil || cache == nil || snap.headState == nil {
		return nil, nil, nil, false, skips
	}
	if estimator == nil {
		estimator = newUnitCostEstimator()
	}
	if maxGroups <= 0 {
		maxGroups = MaxGroupsPerSession
	}

	var newAggregates []*types.SignedAggregatedAttestation
	var payloadEntries []store.PayloadKV
	var keysToDelete []store.AttestationDeleteKey
	truncated := false
	attempted := false
	attempts := 0

	groups := orderedGroups(snap, skips)
	for i, group := range groups {
		// Groups that never reached the prover cost nothing, so the cap counts
		// proof attempts rather than loop iterations.
		if attempts >= maxGroups {
			truncated = true
			skips.addN(metrics.AggGroupSkipSessionCap, len(groups)-i)
			break
		}
		// An over-budget observation must not prevent every future attempt:
		// without a successful proof the estimator cannot recalibrate. Allow
		// one attempt while time remains; subsequent attempts use the estimate.
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 || (attempted && remaining < estimator.nextGroupDuration()) {
				truncated = true
				skips.addN(metrics.AggGroupSkipBudget, len(groups)-i)
				break
			}
		}
		dataRoot := group.dataRoot
		groupStart := time.Now()
		provedBefore := len(newAggregates)
		// Captured from inside the group closure so the estimate can tell a
		// raw-only group from one that carried children.
		groupChildren := 0
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

			// Prefer raw signatures to avoid recursive proving for coverage already
			// available locally. Unlike the spec's child-first selection, children
			// only fill the gaps raw signatures leave.
			//
			// Raw signatures are not rationed. The proof costs what it costs
			// whether it covers two validators or ten, so holding signatures back
			// buys nothing and spends a whole proof on a fraction of the coverage
			// it could have carried. What is rationed is proofs: the per-session
			// group cap and the deadline.
			covered := make(map[uint64]bool)

			if gossipEntry != nil && len(gossipEntry.Signatures) > 0 {
				sortedSigs := make([]store.AttestationSignatureEntry, len(gossipEntry.Signatures))
				copy(sortedSigs, gossipEntry.Signatures)
				sort.Slice(sortedSigs, func(i, j int) bool {
					return sortedSigs[i].ValidatorID < sortedSigs[j].ValidatorID
				})

				for _, sigEntry := range sortedSigs {
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
					covered[sigEntry.ValidatorID] = true
				}
			}

			childBudget := time.Until(deadline)
			childCost := estimator.childDuration()
			childIDs := selectChildProofs(newEntry, snap.headState, childProofsBuf, covered, cache, &childBudget, childCost, len(*rawIDsBuf))
			childIDs = append(childIDs, selectChildProofs(knownEntry, snap.headState, childProofsBuf, covered, cache, &childBudget, childCost, len(*rawIDsBuf))...)
			groupChildren = len(*childProofsBuf)
			childCovered := make(map[uint64]bool, len(childIDs))
			for _, vid := range childIDs {
				childCovered[vid] = true
			}
			kept := 0
			for i, vid := range *rawIDsBuf {
				if childCovered[vid] {
					continue
				}
				(*rawIDsBuf)[kept] = vid
				(*rawPubkeysBuf)[kept] = (*rawPubkeysBuf)[i]
				(*rawSigsBuf)[kept] = (*rawSigsBuf)[i]
				kept++
			}
			*rawIDsBuf = (*rawIDsBuf)[:kept]
			*rawPubkeysBuf = (*rawPubkeysBuf)[:kept]
			*rawSigsBuf = (*rawSigsBuf)[:kept]

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
			attempts++
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

			allIDs := make([]uint64, 0, len(covered))
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
			estimator.observeGroup(time.Since(groupStart), groupChildren)
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
