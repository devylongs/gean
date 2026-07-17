package syncer

import (
	"context"
	"fmt"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/types"
)

func (sd *SyncDriver) checkAndBackfill(ctx context.Context, peerID libp2ppeer.ID, peerStatus *p2p.StatusMessage) {
	if !sd.ready() {
		return
	}
	ctx = sd.contextOrDefault(ctx)

	if !sd.shouldBackfill(peerStatus) {
		return
	}
	if !sd.tryReserve(peerID) {
		return
	}
	defer sd.release(peerID)

	startSlot := sd.store.HeadSlot() + 1
	for startSlot <= peerStatus.HeadSlot {
		if err := ctx.Err(); err != nil {
			return
		}

		count := requestCount(startSlot, peerStatus.HeadSlot)
		logger.Info(logger.Sync, "sync: fetching range from peer %s start_slot=%d count=%d peer_head=%d",
			peerID, startSlot, count, peerStatus.HeadSlot)

		blocks, err := sd.p2p.FetchBlocksByRange(ctx, peerID, startSlot, count)
		if err != nil {
			sd.fallbackHeadByRoot(ctx, peerID, peerStatus, err)
			return
		}
		if len(blocks) == 0 {
			logger.Info(logger.Sync, "sync: peer %s returned empty range at start_slot=%d, stopping", peerID, startSlot)
			return
		}

		lastSlot, ok := sd.feedBlocks(ctx, peerID, blocks, startSlot)
		if !ok {
			return
		}
		startSlot = lastSlot + 1
	}
}

func (sd *SyncDriver) shouldBackfill(peerStatus *p2p.StatusMessage) bool {
	if sd == nil || sd.store == nil || peerStatus == nil {
		return false
	}
	ourHead := sd.store.HeadSlot()
	return peerStatus.HeadSlot > ourHead &&
		peerStatus.HeadSlot-ourHead > blocksByRangeSyncThreshold
}

func requestCount(startSlot, headSlot uint64) uint64 {
	count := headSlot - startSlot + 1
	if count > types.MaxRequestBlocks {
		return types.MaxRequestBlocks
	}
	return count
}

func (sd *SyncDriver) fallbackHeadByRoot(ctx context.Context, peerID libp2ppeer.ID, peerStatus *p2p.StatusMessage, rangeErr error) {
	if !sd.ready() || peerStatus == nil {
		return
	}

	logger.Warn(logger.Sync, "sync: blocks_by_range failed peer=%s err=%v; falling back to head-by-root",
		peerID, rangeErr)

	rootBlocks, _, err := sd.p2p.FetchBlocksByRootBatchWithRetry(ctx, [][32]byte{peerStatus.HeadRoot})
	if err != nil {
		logger.Warn(logger.Sync, "sync: head-by-root fallback also failed: %v", err)
		return
	}
	for _, block := range rootBlocks {
		if validFetchedBlock(block) && !sd.node.OnSyncBlock(ctx, block) {
			return
		}
	}
}

func (sd *SyncDriver) feedBlocks(ctx context.Context, peerID libp2ppeer.ID, blocks []*types.SignedBlock, startSlot uint64) (uint64, bool) {
	if sd == nil || sd.node == nil {
		return 0, false
	}

	logger.Info(logger.Sync, "sync: received %d blocks from peer %s, feeding to engine", len(blocks), peerID)

	lastSlot, ok := validateRangeBlocks(peerID, blocks, startSlot)
	if !ok {
		return 0, false
	}

	// Delivery blocks until the engine accepts each block, so advancing the
	// range cursor past this batch is safe: every slot in it was handed over,
	// not just attempted. Advancing past dropped blocks is what strands a
	// lagging node — the gap is never re-requested and nothing connects.
	for _, block := range blocks {
		if !sd.node.OnSyncBlock(ctx, block) {
			return 0, false
		}
	}
	return lastSlot, true
}

func validateRangeBlocks(peerID libp2ppeer.ID, blocks []*types.SignedBlock, startSlot uint64) (uint64, bool) {
	if len(blocks) == 0 {
		return 0, false
	}

	var lastSlot uint64
	var previousRoot [32]byte
	hasPrevious := false

	for i, block := range blocks {
		if !validFetchedBlock(block) {
			logger.Warn(logger.Sync, "sync: peer %s returned malformed block", peerID)
			return 0, false
		}

		slot := block.Block.Slot
		if slot < startSlot {
			logger.Warn(logger.Sync, "sync: peer %s returned block before requested range slot=%d start_slot=%d",
				peerID, slot, startSlot)
			return 0, false
		}
		if i > 0 && slot <= lastSlot {
			logger.Warn(logger.Sync, "sync: peer %s returned non-monotonic range slot=%d previous_slot=%d",
				peerID, slot, lastSlot)
			return 0, false
		}
		if hasPrevious && block.Block.ParentRoot != previousRoot {
			logger.Warn(logger.Sync, "sync: peer %s returned disconnected range at slot=%d", peerID, slot)
			return 0, false
		}

		root, err := blockHeaderRoot(block.Block)
		if err != nil {
			logger.Warn(logger.Sync, "sync: peer %s returned block with invalid header root at slot=%d: %v",
				peerID, slot, err)
			return 0, false
		}
		previousRoot = root
		hasPrevious = true
		lastSlot = slot
	}
	if lastSlot < startSlot {
		return 0, false
	}
	return lastSlot, true
}

func validFetchedBlock(block *types.SignedBlock) bool {
	return block != nil && block.Block != nil && block.Block.Body != nil
}

func blockHeaderRoot(block *types.Block) ([32]byte, error) {
	if block == nil || block.Body == nil {
		return types.ZeroRoot, fmt.Errorf("block body is nil")
	}
	bodyRoot, err := block.Body.HashTreeRoot()
	if err != nil {
		return types.ZeroRoot, fmt.Errorf("body root: %w", err)
	}
	header := &types.BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     block.StateRoot,
		BodyRoot:      bodyRoot,
	}
	root, err := header.HashTreeRoot()
	if err != nil {
		return types.ZeroRoot, fmt.Errorf("header root: %w", err)
	}
	return root, nil
}
