package aggregation

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

type aggregationGroup struct {
	dataRoot [32]byte
	slot     uint64
}

// orderedGroups lists the snapshot's aggregation work newest-slot-first.
// Fresh attestations are the only ones that can still influence
// justification, so they must be proven inside the session budget; older
// backlog only gets prover time the current slot doesn't need.
func orderedGroups(snap *Snapshot) []aggregationGroup {
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
		groups = append(groups, aggregationGroup{dataRoot: dr, slot: attData.Slot})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].slot != groups[j].slot {
			return groups[i].slot > groups[j].slot
		}
		return bytes.Compare(groups[i].dataRoot[:], groups[j].dataRoot[:]) > 0
	})
	return groups
}

// seedPerUnitSeconds is a conservative starting cost for one aggregation unit
// (one raw signature or one child proof). It only governs the first pass, before
// any real timing is observed: a high seed makes that pass trim rather than risk
// spending the whole session budget on a single group. The estimate then
// converges to the real prover cost of whatever hardware runs the node.
const seedPerUnitSeconds = 0.1

// seedPerGroupSeconds seeds the realized per-group wall-time estimate before any
// group has been observed. It only governs the first group of the first session;
// the estimate then tracks real hardware.
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

// nextGroupDuration estimates the wall time the next group will take (prep plus
// recursive proof). The worker refuses to start a group when less than this
// remains in the budget, so a session cannot overrun and hold the shared prover
// past the slot — the overrun that starved block import and dropped the
// aggregator off the chain at scale.
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

func aggregateFromSnapshot(snap *Snapshot, cache *xmss.PubKeyCache, deadline time.Time, shadowRates shadow.Rates, estimator *unitCostEstimator) ([]*types.SignedAggregatedAttestation, []store.PayloadKV, []store.AttestationDeleteKey, bool) {
	if snap == nil || cache == nil {
		return nil, nil, nil, false
	}
	if estimator == nil {
		estimator = newUnitCostEstimator()
	}

	var newAggregates []*types.SignedAggregatedAttestation
	var payloadEntries []store.PayloadKV
	var keysToDelete []store.AttestationDeleteKey
	truncated := false

	for _, group := range orderedGroups(snap) {
		// Hard budget bound: never start a proof that cannot finish in the time
		// left. A session that overruns keeps the shared prover past the slot and
		// starves block import — the failure that dropped the aggregator off the
		// chain at scale. Stop cleanly and leave this slot's aggregate partial;
		// the deferred groups' signatures remain in the store for the next session.
		if !deadline.IsZero() && time.Until(deadline) < estimator.nextGroupDuration() {
			truncated = true
			break
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
			targetState := snap.targetStates[attData.Target.Root]
			if targetState == nil {
				return
			}

			// Bound this pass to what fits the remaining session budget at the
			// current per-unit estimate. Child proofs go in first (most coverage
			// per unit); fresh raw signatures take whatever budget is left and the
			// rest are deferred to the next pass. A smaller aggregate over the
			// included participants is still spec-valid.
			remaining := estimator.maxUnitsWithin(time.Until(deadline))

			covered := make(map[uint64]bool)
			selectChildProofs(newEntry, targetState, childProofsBuf, covered, cache, &remaining)
			selectChildProofs(knownEntry, targetState, childProofsBuf, covered, cache, &remaining)

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
					if sigEntry.ValidatorID >= uint64(len(targetState.Validators)) {
						continue
					}

					sigHandle := sigEntry.SigHandle
					if sigHandle == nil {
						parsed, err := xmss.ParseSignature(sigEntry.Signature[:])
						if err != nil {
							continue
						}
						defer xmss.FreeSignature(parsed)
						sigHandle = parsed
					}

					pk, err := cache.Get(targetState.Validators[sigEntry.ValidatorID].AttestationPubkey)
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
				return
			}

			dataRootHash, slot, err := aggregationMessage(attData)
			if err != nil {
				logger.Error(logger.Signature, "aggregate: prepare message failed slot=%d: %v", attData.Slot, err)
				return
			}

			metrics.ObserveAggregationPrepTime(time.Since(prepStart).Seconds())

			aggStart := time.Now()
			proofBytes, err := xmss.AggregateWithChildren(*rawPubkeysBuf, *rawSigsBuf, *childProofsBuf, dataRootHash, slot)
			// Charge virtual time for the proving cost Shadow would otherwise not
			// account; the capacity-1 dispatch channel then drops the next slot's
			// work if proving can't keep up, exactly as on real hardware.
			shadowRates.SleepAggregate(len(*rawIDsBuf) + len(*childProofsBuf))
			aggDuration := time.Since(aggStart)
			if err != nil {
				logger.Error(logger.Signature, "aggregate: failed slot=%d raw=%d children=%d duration=%v: %v",
					slot, len(*rawIDsBuf), len(*childProofsBuf), aggDuration, err)
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
		// Only groups that actually proved inform the wall-time estimate; skipped
		// groups (too few signatures) return fast and would bias it low.
		if len(newAggregates) > provedBefore {
			estimator.observeGroup(time.Since(groupStart))
		}
	}

	return newAggregates, payloadEntries, keysToDelete, truncated
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
