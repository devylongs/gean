package node

import (
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/types"
)

func (e *Engine) bufferMissingParentBlock(
	signedBlock *types.SignedBlock,
	blockRoot [32]byte,
	parentRoot [32]byte,
	queue *[]*types.SignedBlock,
) {
	block := signedBlock.Block
	if e.Pending.Count() >= MaxPendingBlocks {
		logger.Warn(logger.Chain, "pending block cache full (%d), rejecting block slot=%d block_root=0x%x",
			MaxPendingBlocks, block.Slot, blockRoot)
		return
	}

	depth := 1
	if parentDepth, ok := e.Pending.Depth(parentRoot); ok {
		depth = parentDepth + 1
	}
	if depth > MaxBlockFetchDepth {
		logger.Warn(logger.Chain, "block fetch depth exceeded (%d > %d), discarding block slot=%d block_root=0x%x",
			depth, MaxBlockFetchDepth, block.Slot, blockRoot)
		return
	}

	logger.Warn(logger.Chain, "block parent missing slot=%d block_root=0x%x parent_root=0x%x depth=%d, storing as pending",
		block.Slot, blockRoot, parentRoot, depth)

	e.Pending.SetDepth(blockRoot, depth)
	missingRoot := e.Pending.ResolveAncestor(parentRoot)
	e.Pending.SetParent(blockRoot, parentRoot)
	e.Store.StorePendingBlock(blockRoot, signedBlock)
	e.Pending.AddChild(parentRoot, blockRoot)

	missingRoot, queued := e.queueStoredAncestor(missingRoot, queue)
	if queued {
		return
	}
	e.queueMissingBlockFetch(missingRoot)
}

func (e *Engine) queueStoredAncestor(missingRoot [32]byte, queue *[]*types.SignedBlock) ([32]byte, bool) {
	for {
		header := e.Store.GetBlockHeader(missingRoot)
		if header == nil {
			return missingRoot, false
		}
		if e.Store.HasState(header.ParentRoot) {
			if storedBlock := e.Store.GetSignedBlock(missingRoot); storedBlock != nil {
				*queue = append(*queue, storedBlock)
			}
			return missingRoot, true
		}
		e.Pending.AddChild(header.ParentRoot, missingRoot)
		e.Pending.SetParent(missingRoot, header.ParentRoot)
		missingRoot = header.ParentRoot
	}
}

func (e *Engine) collectPendingChildren(parentRoot [32]byte, queue *[]*types.SignedBlock) {
	childRoots, ok := e.Pending.RemoveBucket(parentRoot)
	if !ok {
		return
	}

	logger.Info(logger.Chain, "processing %d pending children of parent_root=0x%x", len(childRoots), parentRoot)

	for childRoot := range childRoots {
		e.Pending.ClearEntry(childRoot)

		childBlock := e.Store.GetSignedBlock(childRoot)
		if childBlock == nil {
			logger.Warn(logger.Chain, "pending block block_root=0x%x missing from DB, skipping", childRoot)
			continue
		}
		*queue = append(*queue, childBlock)
	}
}

func (e *Engine) discardFinalizedPending(finalizedSlot uint64) {
	discarded := 0

	for _, pair := range e.Pending.Pairs() {
		parentRoot, childRoot := pair[0], pair[1]
		header := e.Store.GetBlockHeader(childRoot)
		if header != nil && header.Slot <= finalizedSlot {
			e.Pending.DiscardSubtree(childRoot)
			e.Pending.RemoveChild(parentRoot, childRoot)
			discarded++
		}
	}

	if discarded > 0 {
		logger.Info(logger.Store, "discarded %d finalized pending blocks (finalized_slot=%d)", discarded, finalizedSlot)
	}

	if removed := e.PendingAttestations.PruneBelow(finalizedSlot); removed > 0 {
		logger.Info(logger.Store, "discarded %d finalized pending attestations (finalized_slot=%d)", removed, finalizedSlot)
	}
}

func (e *Engine) onFailedRoot(failedRoot [32]byte) {
	// The fetch for this root is exhausted; drop its in-flight marker so it can be
	// re-requested if a later child reintroduces the gap.
	delete(e.fetchInFlight, failedRoot)

	children, ok := e.Pending.RemoveBucket(failedRoot)
	if !ok {
		return
	}

	discarded := 0
	for childRoot := range children {
		e.Pending.DiscardSubtree(childRoot)
		discarded++
	}
	logger.Warn(logger.Sync, "fetch exhausted for root 0x%x, discarded %d pending child block(s)", failedRoot, discarded)
}
