package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
)

// timeEvent records how long one dispatch-loop event took. Every case runs on
// the single goroutine that also keeps the slot clock, so an unattributed slow
// handler here shows up only as a late tick — which is exactly how a set of
// full-table storage scans went unnoticed until they were costing minutes.
func timeEvent(event string, fn func()) {
	start := time.Now()
	fn()
	metrics.ObserveDispatchEvent(event, time.Since(start).Seconds())
}

func (e *Engine) dispatch(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			logger.Info(logger.Node, "shutting down")
			return

		case <-ticks:
			timeEvent("tick", e.onTick)

		case <-e.EarlyAggregateCh:
			timeEvent("early_aggregate", func() { e.maybeEarlyAggregate(uint64(time.Now().UnixMilli())) })

		case block := <-e.BlockCh:
			timeEvent("block", func() { e.onBlock(block) })

		case result := <-e.ProposalResultCh:
			timeEvent("proposal_result", func() { e.acceptProposal(ctx, result) })

		case root := <-e.FailedRootCh:
			timeEvent("failed_root", func() { e.onFailedRoot(root) })
		}
	}
}
