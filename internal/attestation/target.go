package attestation

import (
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func GetAttestationTarget(s *store.ConsensusStore) *types.Checkpoint {
	targetRoot := s.Head()
	targetHeader := s.GetBlockHeader(targetRoot)
	if targetHeader == nil {
		return &types.Checkpoint{}
	}

	safeTargetSlot := uint64(0)
	if safeTargetHeader := s.GetBlockHeader(s.SafeTarget()); safeTargetHeader != nil {
		safeTargetSlot = safeTargetHeader.Slot
	}

	// The walk never crosses the finalized boundary: a safe target lagging
	// behind finalization falls back to the finalized slot as the lower bound.
	finalizedSlot := s.LatestFinalized().Slot
	lowerBoundSlot := safeTargetSlot
	if finalizedSlot > lowerBoundSlot {
		lowerBoundSlot = finalizedSlot
	}

	for range uint64(types.JustificationLookbackSlots) {
		if targetHeader.Slot <= lowerBoundSlot {
			break
		}
		targetRoot = targetHeader.ParentRoot
		parent := s.GetBlockHeader(targetRoot)
		if parent == nil {
			break
		}
		targetHeader = parent
	}

	for targetHeader.Slot > finalizedSlot &&
		!statetransition.SlotIsJustifiableAfter(targetHeader.Slot, finalizedSlot) {
		targetRoot = targetHeader.ParentRoot
		parent := s.GetBlockHeader(targetRoot)
		if parent == nil {
			break
		}
		targetHeader = parent
	}

	return &types.Checkpoint{
		Root: targetRoot,
		Slot: targetHeader.Slot,
	}
}
