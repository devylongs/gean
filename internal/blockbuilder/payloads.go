package blockbuilder

import (
	"sort"

	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/types"
)

func sortedPayloads(payloads []AttestationPayload) ([]AttestationPayload, []PayloadError) {
	byRoot := make(map[[32]byte]AttestationPayload, len(payloads))
	var payloadErrors []PayloadError
	for _, payload := range payloads {
		if err := validatePayload(payload); err != nil {
			payloadErrors = append(payloadErrors, PayloadError{
				DataRoot: payload.DataRoot,
				Err:      err,
			})
			continue
		}
		computedRoot, err := payload.Data.HashTreeRoot()
		if err != nil {
			payloadErrors = append(payloadErrors, PayloadError{
				DataRoot: payload.DataRoot,
				Err:      err,
			})
			continue
		}
		if computedRoot != payload.DataRoot {
			payloadErrors = append(payloadErrors, PayloadError{
				DataRoot: payload.DataRoot,
				Err:      errPayloadRootMismatch(payload.DataRoot, computedRoot),
			})
			continue
		}
		if existing, ok := byRoot[payload.DataRoot]; ok {
			existing.Proofs = append(existing.Proofs, payload.Proofs...)
			byRoot[payload.DataRoot] = existing
			continue
		}
		payload.Proofs = append([]*types.SingleMessageAggregate(nil), payload.Proofs...)
		byRoot[payload.DataRoot] = payload
	}

	sorted := make([]AttestationPayload, 0, len(byRoot))
	for _, payload := range byRoot {
		sorted = append(sorted, payload)
	}
	sort.Slice(sorted, func(i, j int) bool {
		left := sorted[i].Data.Target.Slot
		right := sorted[j].Data.Target.Slot
		if left != right {
			return left < right
		}
		return compareRoots(sorted[i].DataRoot, sorted[j].DataRoot) < 0
	})
	return sorted, payloadErrors
}

func validatePayload(payload AttestationPayload) error {
	if payload.Data == nil {
		return errMalformedPayload("data is nil")
	}
	if len(payload.Proofs) == 0 {
		return errMalformedPayload("proofs are empty")
	}
	if payload.Data.Head == nil {
		return errMalformedPayload("head checkpoint is nil")
	}
	if payload.Data.Target == nil {
		return errMalformedPayload("target checkpoint is nil")
	}
	if payload.Data.Source == nil {
		return errMalformedPayload("source checkpoint is nil")
	}
	return nil
}

func payloadBuildIssue(state *types.State, knownRoots KnownRoots, payload AttestationPayload) error {
	data := payload.Data
	if !knownRoots.Contains(data.Head.Root) {
		return errPayloadHeadUnknown(data.Head.Root)
	}
	// Only source votes from the chain's current justified checkpoint: older sources
	// anchor off a different chain; newer ones are retried once a later pass justifies them.
	if justified := currentJustifiedSlot(state); data.Source.Slot != justified {
		return errPayloadSourceNotCurrentJustified(data.Source.Slot, justified)
	}
	// Head must sit on the canonical chain, not merely be known, so the builder drops
	// the same off-fork votes the state transition would skip.
	if !statetransition.HeadMatchesChain(state, data.Head) {
		return errPayloadHeadOffChain(data.Head.Root)
	}
	// An out-of-range justified-slot query would reject the block being built; drop the vote.
	reason, err := statetransition.VoteInvalidReason(state, data.Source, data.Target)
	if err != nil {
		return errPayloadVoteInvalid(data, err.Error())
	}
	if reason != "" {
		if data.Source.Slot == 0 && data.Target.Slot == 0 &&
			reason == statetransition.VoteReasonTargetAlreadyJustified {
			return nil
		}
		return errPayloadVoteInvalid(data, reason)
	}
	return nil
}

// currentJustifiedSlot is the working state's justified slot — the only source
// slot eligible this pass — advancing as the fixed-point loop accepts votes.
func currentJustifiedSlot(state *types.State) uint64 {
	if state == nil || state.LatestJustified == nil {
		return 0
	}
	return state.LatestJustified.Slot
}

func compareRoots(a, b [32]byte) int {
	for i := range 32 {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
