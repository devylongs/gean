package statetransition

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func ProcessAttestations(state *types.State, attestations []*types.AggregatedAttestation) error {
	if state == nil {
		return malformedState("state")
	}
	if state.LatestJustified == nil {
		return malformedState("latest justified")
	}
	if state.LatestFinalized == nil {
		return malformedState("latest finalized")
	}

	// Bound the distinct votes the block may carry. The cap belongs to the
	// transition itself, not to any one caller, so the raw transition and
	// block-production trial blocks inherit the bound. Each distinct data builds a
	// tally sized to the validator set, so an unbounded count amplifies import
	// work; the SSZ list limit sits far above the consensus cap and cannot
	// substitute. Only the distinct count is bounded: split aggregates that share
	// one data entry count once.
	distinct := make(map[[32]byte]struct{})
	for _, agg := range attestations {
		if agg == nil || agg.Data == nil {
			continue
		}
		dataRoot, err := agg.Data.HashTreeRoot()
		if err != nil {
			return fmt.Errorf("hash attestation data: %w", err)
		}
		distinct[dataRoot] = struct{}{}
	}
	if len(distinct) > int(types.MaxAttestationsData) {
		return &TooManyAttestationDataError{Count: uint64(len(distinct)), Max: uint64(types.MaxAttestationsData)}
	}

	validatorCount := int(state.NumValidators())
	if validatorCount == 0 {
		return ErrEmptyValidatorRegistry
	}

	// The flat vote bitlist segments into one block of validatorCount bits per
	// tracked root; a mismatched length means the state is malformed and cannot be
	// interpreted, so reject rather than read past or short of a segment.
	if int(types.BitlistLen(state.JustificationsValidators)) != len(state.JustificationsRoots)*validatorCount {
		return ErrJustificationVotesLengthMismatch
	}

	for _, root := range state.JustificationsRoots {
		var r [32]byte
		copy(r[:], root)
		if types.IsZeroRoot(r) {
			return ErrZeroHashInJustificationRoots
		}
	}

	justifications := reconstructJustifications(state, validatorCount)
	rootToSlot := buildRootToSlot(state)

	for _, agg := range attestations {
		if !validAttestationShape(agg) {
			continue
		}
		source := agg.Data.Source
		target := agg.Data.Target

		// An out-of-range justified-slot query rejects the whole block; any other
		// invalid reason just filters the vote out, matching leanSpec.
		reason, err := VoteInvalidReason(state, source, target)
		if err != nil {
			return err
		}
		if reason != "" {
			continue
		}
		if !HeadMatchesChain(state, agg.Data.Head) {
			continue
		}

		// An attestation that reaches the tally must name at least one
		// in-range voter; empty bits or a set bit outside the registry
		// reject the whole block. Unset padding past the registry is
		// harmless. This guards the unsigned path, which has no signature
		// stage to reject such bits first.
		voterIndices := types.BitlistIndices(agg.AggregationBits)
		if len(voterIndices) == 0 {
			return ErrEmptyAggregationBits
		}
		for _, voter := range voterIndices {
			if voter >= uint64(validatorCount) {
				return &AttesterIndexOutOfRangeError{Index: voter, Validators: uint64(validatorCount)}
			}
		}

		votes, exists := justifications[target.Root]
		if !exists {
			votes = make([]bool, validatorCount)
			justifications[target.Root] = votes
		}

		for _, voter := range voterIndices {
			votes[voter] = true
		}

		voteCount := countTrue(votes)
		if 3*voteCount >= 2*validatorCount {
			if target.Slot > state.LatestJustified.Slot {
				state.LatestJustified = copyCheckpoint(target)
			}
			setSlotJustified(state, state.LatestFinalized.Slot, target.Slot)

			delete(justifications, target.Root)
			tryFinalize(state, source, target, &justifications, rootToSlot)
		}
	}

	serializeJustifications(state, justifications, validatorCount)

	return nil
}
