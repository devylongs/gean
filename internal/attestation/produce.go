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
	// The target walk is bounded by the store's safe target, which can momentarily lag
	// the head state's justified checkpoint while the node catches up on imports. When it
	// does, the walk stops below the source; clamp the target up to the source so the vote
	// stays valid (source <= target, the spec's produce_attestation_data invariant) rather
	// than dropping the attestation and starving fork choice of the head vote.
	if source.Slot > target.Slot {
		target = &types.Checkpoint{Root: source.Root, Slot: source.Slot}
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
