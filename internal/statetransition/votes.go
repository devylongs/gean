package statetransition

import "github.com/geanlabs/gean/internal/types"

func validAttestationShape(agg *types.AggregatedAttestation) bool {
	return agg != nil && agg.Data != nil && agg.Data.Source != nil && agg.Data.Target != nil
}

const (
	VoteReasonNilInput               = "nil_input"
	VoteReasonSourceNotJustified     = "source_not_justified"
	VoteReasonTargetAlreadyJustified = "target_already_justified"
	VoteReasonZeroRoot               = "zero_root"
	VoteReasonChainMismatch          = "chain_mismatch"
	VoteReasonTargetNotAfterSource   = "target_not_after_source"
	VoteReasonTargetNotJustifiable   = "target_not_justifiable"
)

// VoteInvalidReason classifies a vote for filtering. A non-empty reason means the
// vote is skipped; a non-nil error is a hard block rejection. The justification
// queries are ordered ahead of the chain checks to mirror leanSpec
// process_attestations, where an out-of-range source or target slot rejects the
// block before the chain-membership filter runs.
func VoteInvalidReason(state *types.State, source, target *types.Checkpoint) (string, error) {
	if state == nil || state.LatestFinalized == nil || source == nil || target == nil {
		return VoteReasonNilInput, nil
	}

	finalizedSlot := state.LatestFinalized.Slot

	sourceJustified, err := IsSlotJustified(state, finalizedSlot, source.Slot)
	if err != nil {
		return "", err
	}
	if !sourceJustified {
		return VoteReasonSourceNotJustified, nil
	}

	targetJustified, err := IsSlotJustified(state, finalizedSlot, target.Slot)
	if err != nil {
		return "", err
	}
	if targetJustified {
		return VoteReasonTargetAlreadyJustified, nil
	}

	if types.IsZeroRoot(source.Root) || types.IsZeroRoot(target.Root) {
		return VoteReasonZeroRoot, nil
	}
	if !checkpointExists(state, source) || !checkpointExists(state, target) {
		return VoteReasonChainMismatch, nil
	}
	if target.Slot <= source.Slot {
		return VoteReasonTargetNotAfterSource, nil
	}
	if !SlotIsJustifiableAfter(target.Slot, finalizedSlot) {
		return VoteReasonTargetNotJustifiable, nil
	}
	return "", nil
}

func IsValidVote(state *types.State, source, target *types.Checkpoint) bool {
	reason, err := VoteInvalidReason(state, source, target)
	return err == nil && reason == ""
}

// HeadMatchesChain reports whether the attestation head sits on the canonical
// chain at its slot. Mirrors the head clause of leanSpec attestation_data_matches_chain;
// applied both in process_attestations and in block production.
func HeadMatchesChain(state *types.State, head *types.Checkpoint) bool {
	return head != nil && !types.IsZeroRoot(head.Root) && checkpointExists(state, head)
}

// IsSlotJustified reports whether a slot is justified. Slots at or below the
// finalized boundary are justified by definition. An active slot beyond the tracked
// bitfield is not a "false" answer but a domain rejection: leanSpec surfaces it as
// JUSTIFIED_SLOT_OUT_OF_RANGE rather than letting it pass as an unjustified vote.
func IsSlotJustified(state *types.State, finalizedSlot, slot uint64) (bool, error) {
	if slot <= finalizedSlot {
		return true, nil
	}
	if state == nil {
		return false, nil
	}
	relIndex := slot - finalizedSlot - 1
	trackedLen := types.BitlistLen(state.JustifiedSlots)
	if relIndex >= trackedLen {
		return false, &JustifiedSlotOutOfRangeError{
			Slot:              slot,
			FinalizedBoundary: finalizedSlot,
			TrackedLength:     trackedLen,
		}
	}
	return types.BitlistGet(state.JustifiedSlots, relIndex), nil
}

func setSlotJustified(state *types.State, finalizedSlot, slot uint64) {
	if state == nil || slot <= finalizedSlot {
		return
	}
	relIndex := slot - finalizedSlot - 1
	if relIndex >= types.BitlistLen(state.JustifiedSlots) {
		state.JustifiedSlots = types.BitlistExtend(state.JustifiedSlots, relIndex+1)
	}
	types.BitlistSet(state.JustifiedSlots, relIndex)
}

func checkpointExists(state *types.State, cp *types.Checkpoint) bool {
	if state == nil || cp == nil {
		return false
	}
	if cp.Slot >= uint64(len(state.HistoricalBlockHashes)) {
		return false
	}
	var stored [32]byte
	copy(stored[:], state.HistoricalBlockHashes[cp.Slot])
	return stored == cp.Root
}

func countTrue(votes []bool) int {
	count := 0
	for _, v := range votes {
		if v {
			count++
		}
	}
	return count
}
