package aggregation

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/shadowcost"
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

func RunWorker(
	ctx context.Context,
	dispatches <-chan Dispatch,
	consensusStore *store.ConsensusStore,
	cache *xmss.PubKeyCache,
	publisher Publisher,
	costs shadowcost.Costs,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case dispatch, ok := <-dispatches:
			if !ok {
				return
			}
			if dispatch.Snapshot == nil {
				continue
			}

			workerStart := time.Now()
			aggs, payloads, deletes := aggregateFromSnapshot(dispatch.Snapshot, cache)
			costs.DelayAggregate()
			applyAggregationMutations(consensusStore, payloads, deletes)
			publishAggregates(ctx, publisher, aggs)
			metrics.ObserveAggregationWorkerTotalTime(time.Since(workerStart).Seconds())
			logger.Info(logger.Signature, "aggregation worker: slot=%d produced=%d duration=%v",
				dispatch.Slot, len(aggs), time.Since(workerStart))
		}
	}
}
