package attestation

import (
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func ProduceAttestationData(s *store.ConsensusStore, slot uint64) *types.AttestationData {
	headRoot := s.Head()
	headState := s.GetState(headRoot)
	if headState == nil || headState.LatestBlockHeader == nil || headState.LatestJustified == nil {
		return nil
	}

	headHeader := s.GetBlockHeader(headRoot)
	if headHeader == nil {
		return nil
	}
	headCheckpoint := &types.Checkpoint{
		Root: headRoot,
		Slot: headHeader.Slot,
	}

	target := GetAttestationTarget(s)

	// Source from the head chain's own justified checkpoint, not the store's global
	// one: the store may have justified on a minority fork the head never extended.
	justified := headState.LatestJustified
	source := &types.Checkpoint{Root: justified.Root, Slot: justified.Slot}
	// Genesis justified root is the zero placeholder; resolve it to a real block.
	if types.IsZeroRoot(source.Root) {
		source.Root = headRoot
	}
	if source.Slot > target.Slot {
		logger.Error(logger.Chain, "ProduceAttestation: source slot %d exceeds target slot %d", source.Slot, target.Slot)
		return nil
	}

	logger.Info(logger.Chain, "ProduceAttestation: slot=%d head=0x%x source=0x%x/%d target=0x%x/%d",
		slot, headRoot, source.Root, source.Slot, target.Root, target.Slot)

	return &types.AttestationData{
		Slot:   slot,
		Head:   headCheckpoint,
		Target: target,
		Source: source,
	}
}
