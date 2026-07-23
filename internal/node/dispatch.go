package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/logger"
)

func (e *Engine) dispatch(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			logger.Info(logger.Node, "shutting down")
			return

		case <-ticks:
			e.onTick()

		case block := <-e.BlockCh:
			e.onBlock(block)

		case agg := <-e.AggregationCh:
			e.onGossipAggregatedAttestation(agg)

		case result := <-e.ProposalResultCh:
			e.acceptProposal(ctx, result)

		case root := <-e.FailedRootCh:
			e.onFailedRoot(root)
		}
	}
}
