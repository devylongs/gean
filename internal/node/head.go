package node

import (
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func (e *Engine) updateHead() {
	attestations := e.Store.ExtractLatestKnownAttestations()
	justifiedRoot := e.Store.LatestJustified().Root

	// FindHead echoes back a justified root it doesn't know, freezing the
	// head there — surface that state instead of stalling silently.
	if e.FC.NodeIndex(justifiedRoot) < 0 && e.warnedMissingJustified != justifiedRoot {
		e.warnedMissingJustified = justifiedRoot
		logger.Warn(logger.Forkchoice, "justified root 0x%x unknown to fork choice; head cannot advance past it", justifiedRoot)
	}

	for vid, data := range attestations {
		e.FC.SetKnownVote(vid, data.Head.Root, data.Slot, data)
	}

	oldHead := e.Store.Head()
	newHead := e.FC.UpdateHead(justifiedRoot)

	e.updateFinalizedFromHead(newHead)

	if newHead != oldHead {
		e.Store.SetHead(newHead)
		if !types.IsZeroRoot(oldHead) {
			newHeader := e.Store.GetBlockHeader(newHead)
			if newHeader == nil {
				return
			}
			justified := e.Store.LatestJustified()
			finalized := e.Store.LatestFinalized()

			isReorg := newHeader.ParentRoot != oldHead

			metrics.SetHeadSlot(newHeader.Slot)
			metrics.SetLatestJustifiedSlot(justified.Slot)
			metrics.SetLatestFinalizedSlot(finalized.Slot)
			metrics.SetJustifiedSlot(justified.Slot)
			metrics.SetFinalizedSlot(finalized.Slot)
			metrics.SetGossipSignatures(e.Store.AttestationSignatures.Len())
			metrics.SetNewAggregatedPayloads(e.Store.NewPayloads.Len())
			metrics.SetKnownAggregatedPayloads(e.Store.KnownPayloads.Len())
			metrics.SetPendingAttestationsTotal(e.PendingAttestations.Total())

			if isReorg {
				metrics.IncForkChoiceReorgs()
				depth := e.FC.ReorgDepth(oldHead, newHead)
				metrics.ObserveForkChoiceReorgDepth(float64(depth))
				logger.Warn(logger.Forkchoice, "REORG depth=%d slot=%d head_root=0x%x parent_root=0x%x (was 0x%x) justified_slot=%d justified_root=0x%x finalized_slot=%d finalized_root=0x%x",
					depth, newHeader.Slot, newHead, newHeader.ParentRoot, oldHead,
					justified.Slot, justified.Root,
					finalized.Slot, finalized.Root)
			} else {
				logger.Info(logger.Forkchoice, "head slot=%d head_root=0x%x parent_root=0x%x justified_slot=%d justified_root=0x%x finalized_slot=%d finalized_root=0x%x",
					newHeader.Slot, newHead, newHeader.ParentRoot,
					justified.Slot, justified.Root,
					finalized.Slot, finalized.Root)
			}
		}
	}
}

// updateFinalizedFromHead keeps the finalized checkpoint on the canonical head's
// chain. The finalized slot is taken from the head's post-state and re-anchored to
// the head's ancestor at that slot, rather than advanced independently per imported
// block: a losing fork that finalizes a higher slot must not latch finalization
// above the canonical head, which would otherwise stall target advancement. When
// finalization advances it drives the same pruning/discard work the import path used.
func (e *Engine) updateFinalizedFromHead(headRoot [32]byte) {
	derived := store.DeriveFinalizedFromHead(e.Store, headRoot)
	if derived == nil {
		return
	}

	old := e.Store.LatestFinalized()
	oldSlot := uint64(0)
	if old != nil {
		oldSlot = old.Slot
	}
	if derived.Slot <= oldSlot {
		return
	}

	e.Store.SetLatestFinalized(derived)
	metrics.IncFinalization("success")
	logger.Info(logger.Forkchoice, "finalized advanced slot=%d root=0x%x", derived.Slot, derived.Root)

	if derived.Slot > 0 {
		e.FC.Prune(derived.Root)
	}
	store.PruneOnFinalization(e.Store, e.FC, oldSlot, derived.Slot, derived.Root)
	e.discardFinalizedPending(derived.Slot)
}

func (e *Engine) updateSafeTarget() {
	attestations := e.Store.ExtractLatestNewAttestations()
	justifiedRoot := e.Store.LatestJustified().Root

	for vid, data := range attestations {
		e.FC.SetNewVote(vid, data.Head.Root, data.Slot, data)
	}

	headState := e.Store.GetState(e.Store.Head())
	if headState == nil {
		return
	}
	numValidators := uint64(len(headState.Validators))

	safeTarget := e.FC.UpdateSafeTarget(justifiedRoot, numValidators)
	e.Store.SetSafeTarget(safeTarget)

	safeHeader := e.Store.GetBlockHeader(safeTarget)
	if safeHeader != nil {
		metrics.SetSafeTargetSlot(safeHeader.Slot)
	}
}
