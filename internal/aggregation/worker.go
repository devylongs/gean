package aggregation

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/proving"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

type Publisher interface {
	PublishAggregatedAttestation(context.Context, *types.SignedAggregatedAttestation) error
}

type Dispatch struct {
	Snapshot *Snapshot
	Slot     uint64
}

// SessionBudget caps one aggregation session's proving time. Dispatch fires
// at interval 2, so two intervals of proving still leaves interval 4 for the
// results to be promoted and gossiped, and bounds how long the proving gate
// is held away from proposals. Exported so other prover users can stay clear
// of the window this session occupies rather than fork the constant.
const SessionBudget = 2 * types.MillisecondsPerInterval * time.Millisecond

// AcquirePatience is how long a dispatched session waits for the prover before
// giving up on the slot's aggregate.
const AcquirePatience = 750 * time.Millisecond

func RunWorker(
	ctx context.Context,
	dispatches <-chan Dispatch,
	consensusStore *store.ConsensusStore,
	cache *xmss.PubKeyCache,
	publisher Publisher,
	gate *proving.Gate,
	shadowRates shadow.Rates,
) {
	// One estimator lives across dispatches so it keeps calibrating to this
	// node's real per-unit prover cost. The worker is single-threaded, so no
	// locking is needed.
	estimator := newUnitCostEstimator()
	for {
		select {
		case <-ctx.Done():
			return
		case dispatch, ok := <-dispatches:
			if !ok {
				return
			}
			metrics.SetProvingQueueDepth("aggregation", len(dispatches))
			if dispatch.Snapshot == nil {
				continue
			}
			acquireCtx, cancelAcquire := context.WithTimeout(ctx, AcquirePatience)
			if gate != nil && !gate.Acquire(acquireCtx, false) {
				cancelAcquire()
				metrics.IncProofOperation("aggregation", "canceled")
				// A lost prover costs this slot its aggregate and slows justification;
				// without this line the loss shows only in metrics, a silent gap in the logs.
				logger.Warn(logger.Signature, "aggregation skipped: prover unavailable slot=%d", dispatch.Slot)
				continue
			}
			cancelAcquire()

			// The session budget bounds how long the gate is held, not which
			// results survive: groups are proven newest-first and every
			// completed aggregate is applied and published even when the
			// budget cuts the session short. Discarding finished aggregates
			// (and their signature deletes) regrows the next snapshot until
			// no session can ever finish inside a slot.
			workerStart := time.Now()
			aggs, payloads, deletes, truncated := aggregateFromSnapshot(dispatch.Snapshot, cache, workerStart.Add(SessionBudget), shadowRates, estimator)
			if gate != nil {
				gate.Release(false)
			}
			if truncated {
				metrics.IncProofOperation("aggregation", "truncated")
				logger.Warn(logger.Signature, "aggregation session hit budget: slot=%d produced=%d", dispatch.Slot, len(aggs))
			}
			applyAggregationMutations(consensusStore, payloads, deletes)
			publishCtx, cancelPublish := context.WithTimeout(ctx, types.MillisecondsPerInterval*time.Millisecond)
			publishAggregates(publishCtx, publisher, aggs)
			cancelPublish()
			metrics.IncProofOperation("aggregation", "success")
			metrics.ObserveProvingDuration("aggregation", time.Since(workerStart).Seconds())
			metrics.ObserveAggregationWorkerTotalTime(time.Since(workerStart).Seconds())
			logger.Info(logger.Signature, "aggregation worker: slot=%d produced=%d duration=%v",
				dispatch.Slot, len(aggs), time.Since(workerStart))
		}
	}
}
