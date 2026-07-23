package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/types"
)

func (e *Engine) OnBlock(block *types.SignedBlock) {
	e.noteGossipSlot(block)
	select {
	case e.BlockCh <- block:
	default:
		logger.Warn(logger.Chain, "block channel full, dropping")
	}
}

// noteGossipSlot records the highest slot heard on gossip as evidence that the
// network is producing blocks — counted before admission, because a node that
// drops or rejects what it hears must not mistake its own import stall for a
// network-wide one. Far-future slots are ignored so a hostile peer cannot pin
// the duty gate shut with a fabricated slot.
func (e *Engine) noteGossipSlot(block *types.SignedBlock) {
	if block == nil || block.Block == nil {
		return
	}
	slot := block.Block.Slot
	if slot > e.currentSlot(uint64(time.Now().UnixMilli()))+1 {
		return
	}
	for {
		cur := e.maxSeenGossipSlot.Load()
		if slot <= cur || e.maxSeenGossipSlot.CompareAndSwap(cur, slot) {
			return
		}
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
