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
	e.refreshGossipMeshPeers()

	isAgg := e.AggCtl != nil && e.AggCtl.Get()

	hasProposal := false
	var proposerValidatorID uint64
	if currentInterval == 0 && currentSlot > 0 && !firstTick {
		proposerValidatorID, hasProposal = e.getOurProposer(currentSlot)
	}

	store.OnTick(e.Store, timestampMs, hasProposal)

	if currentInterval == 2 {
		// Dispatch unconditionally, matching leanSpec's interval-2 aggregation.
		// Contention with an upcoming proposal duty is handled by the proving
		// gate's proposal priority, not by skipping the cycle: a sole aggregator
		// that also proposes next would otherwise never aggregate at all.
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
	if e.Store.GetState(e.Store.Head()) == nil {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipMissingState)
		return
	}

	snap := aggregation.SnapshotInputs(e.Store)
	if snap == nil {
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipOther)
		return
	}
	select {
	case e.AggregationDispatchCh <- aggregation.Dispatch{Snapshot: snap, Slot: currentSlot}:
		metrics.SetProvingQueueDepth("aggregation", len(e.AggregationDispatchCh))
	default:
		metrics.IncAggregationDispatchDropped()
		metrics.IncAggregatorSkipped(metrics.AggregatorSkipSpawnFailed)
	}
}

func (e *Engine) runAttestationInterval(currentSlot uint64) {
	e.drainPendingBlocks()
	e.updateHead()
	e.produceAttestations(currentSlot)
	e.logChainStatus(currentSlot)
}
