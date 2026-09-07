package node

import (
	"time"

	"github.com/geanlabs/gean/internal/aggregation"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
)

func (e *Engine) onTick() {
	now := time.Now()
	firstTick := e.lastTick.IsZero()
	if !firstTick {
		metrics.ObserveTickIntervalDuration(now.Sub(e.lastTick).Seconds())
	}
	e.lastTick = now

	timestampMs := uint64(now.UnixMilli())

	currentSlot := e.currentSlot(timestampMs)
	currentInterval := e.currentInterval(timestampMs)

	metrics.SetCurrentSlot(currentSlot)
	e.updateSyncStatus(currentSlot)

	isAgg := e.AggCtl != nil && e.AggCtl.Get()

	hasProposal := false
	var proposerValidatorID uint64
	if currentInterval == 0 && currentSlot > 0 && !firstTick {
		proposerValidatorID, hasProposal = e.getOurProposer(currentSlot)
	}

	// Capture before OnTick promotes new payloads into known, so the timely
	// section reflects what had arrived by the promotion boundary.
	if snap := snapshotNewPayloadParticipants(e.Store); snap != nil {
		e.coveragePreMerge = snap
	}

	store.OnTick(e.Store, timestampMs, hasProposal)

	if currentInterval == 2 {
		e.reportAggStartNewCoverage()
		// Dispatch unconditionally, matching leanSpec's interval-2 aggregation:
		// a sole aggregator that also proposes next would otherwise never
		// aggregate at all. The proving gate's proposal priority only defers
		// the *next* background acquire; it cannot preempt a session already
		// holding the token, so a proposal duty landing mid-session waits for
		// the whole session. What bounds that wait is the per-session group
		// cap, not the gate.
		e.dispatchAggregationCycle(currentSlot, isAgg)
	}

	if currentInterval == 0 || currentInterval == 4 {
		e.updateHead()
	}

	if hasProposal {
		e.maybePropose(currentSlot, proposerValidatorID)
	}

	if currentInterval == 1 {
		e.runAttestationInterval(currentSlot)
	}

	if currentInterval == 3 {
		e.updateSafeTarget()
		store.PeriodicPrune(e.Store, e.FC, currentSlot, e.Store.LatestFinalized().Slot)
	}
}

func (e *Engine) dispatchAggregationCycle(currentSlot uint64, isAggregator bool) {
	if !isAggregator {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipNotAggregator)
		return
	}
	// Dispatch at most once per slot: the early attestation-arrival path and the
	// interval-2 fallback both route here, and the recursive proof is far too
	// expensive to run twice for the same slot.
	if currentSlot == e.aggregatedSlot {
		return
	}
	// The sync-lag duty gate is spec-defined only for block and attestation; gean
	// also applies it to aggregation. Aggregating on a stale view only produces
	// best-effort aggregates that get dropped, so gating when lagging is safe and
	// surfaces the not_synced skip reason.
	if e.DutyGate != nil && !e.DutyGate.Decide("aggregation", currentSlot, e.Store.HeadSlot(), e.networkSeenSlot()) {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipNotSynced)
		return
	}
	if e.Store.AttestationSignatures.Len() == 0 && e.Store.NewPayloads.Len() == 0 {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipOther)
		return
	}
	headState := e.Store.GetState(e.Store.Head())
	if headState == nil {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipMissingState)
		return
	}

	snap := aggregation.SnapshotInputs(e.Store)
	if snap == nil {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipOther)
		return
	}
	// A session holds the proving gate until it finishes, so a proposal duty
	// next slot waits on it however the gate's priority flag is set. Prove one
	// group in that case and leave the rest for the following session.
	maxGroups := aggregation.MaxGroupsPerSession
	if e.proposingAt(currentSlot+1, headState.NumValidators()) {
		maxGroups = aggregation.MaxGroupsWhenProposing
	}
	select {
	case e.AggregationDispatchCh <- aggregation.Dispatch{Snapshot: snap, Slot: currentSlot, MaxGroups: maxGroups}:
		e.aggregatedSlot = currentSlot
		metrics.SetProvingQueueDepth("aggregation", len(e.AggregationDispatchCh))
	default:
		metrics.IncAggregationDispatchDropped()
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipSpawnFailed)
	}
}

// maybeEarlyAggregate starts the aggregation session in late interval 1, once this
// slot's votes have reached quorum — a partial-interval lead ahead of the interval-2
// fallback — so the slow recursive proof gets a head start toward finishing inside
// its budget instead of racing the interval boundary and truncating. Gating on the
// slot's own vote count rather than a wall-clock lead keeps the timing tied to the
// slot model. It only moves *when the proving starts*: the aggregate still covers
// same-slot attestations and is gossiped within the slot, so it is spec-neutral.
// dispatchAggregationCycle enforces the once-per-slot guard shared with the
// interval-2 fallback.
func (e *Engine) maybeEarlyAggregate(nowMs uint64) {
	isAgg := e.AggCtl != nil && e.AggCtl.Get()
	if !isAgg || e.currentInterval(nowMs) != 1 {
		return
	}
	slot := e.currentSlot(nowMs)
	if slot == e.aggregatedSlot {
		return
	}
	// Validator set is fixed at genesis in lean devnet, so decode the head state
	// once and cache the count rather than on every arrival.
	if e.numValidators == 0 {
		headState := e.Store.GetState(e.Store.Head())
		if headState == nil {
			return
		}
		e.numValidators = headState.NumValidators()
		if e.numValidators == 0 {
			return
		}
	}
	// Only pull the session forward once a finalizing supermajority of *this slot's*
	// votes is already collected. The count must be scoped to the current slot: a
	// cross-slot backlog would satisfy the threshold at the very start of interval 1,
	// before the slot's own attestations have propagated, and bundling then starves
	// justification. Scoped to the slot, the threshold is reached only in late
	// interval 1 once votes are in — a modest proving lead that still carries
	// decisive weight, with the interval-2 dispatch as the fallback below quorum.
	if e.Store.AttestationSignatures.SignatureCountForSlot(slot) < earlyAggregationQuorum(e.numValidators) {
		return
	}
	e.dispatchAggregationCycle(slot, isAgg)
}

// earlyAggregationQuorum is the 3SF supermajority ceil(2n/3) — the same threshold
// the fork choice uses for justification. At that many collected votes an early
// aggregate already carries finalizing weight, so proving it ahead of interval 2
// is worthwhile.
func earlyAggregationQuorum(numValidators uint64) int {
	return int((2*numValidators + 2) / 3)
}

func (e *Engine) runAttestationInterval(currentSlot uint64) {
	e.drainPendingBlocks()
	e.updateHead()
	e.produceAttestations(currentSlot)
	// Report the previous round: by now the head normally carries the block
	// proposed at currentSlot, which is the first block able to include votes
	// for currentSlot-1.
	if currentSlot > 0 {
		e.reportPostBlockCoverage(currentSlot - 1)
	}
	e.logChainStatus(currentSlot)
}
