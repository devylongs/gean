package node

import (
	"context"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/types"
)

func (e *Engine) OnBlock(block *types.SignedBlock) {
	select {
	case e.BlockCh <- block:
	default:
		logger.Warn(logger.Chain, "block channel full, dropping")
	}
}

// OnSyncBlock delivers a block this node itself requested (range backfill or
// by-root parent fetch). Unlike gossip delivery it blocks until the dispatch
// loop accepts the block: a requested block dropped on the floor is a gap the
// requester believes it already covered, so it is never re-requested and the
// chain can no longer connect. Returns false only if ctx ends first.
func (e *Engine) OnSyncBlock(ctx context.Context, block *types.SignedBlock) bool {
	select {
	case e.BlockCh <- block:
		return true
	case <-ctx.Done():
		return false
	}
}

func (e *Engine) OnGossipAttestation(att *types.SignedAttestation) {
	select {
	case e.AttestationCh <- att:
	default:
		logger.Warn(logger.Gossip, "attestation channel full, dropping")
	}
}

func (e *Engine) OnGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation) {
	select {
	case e.AggregationCh <- agg:
	default:
		logger.Warn(logger.Signature, "aggregation channel full, dropping")
	}
}
