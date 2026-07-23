package attestationproof

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

func Select(
	data *types.AttestationData,
	proofs []*types.SingleMessageAggregate,
	state *types.State,
	merger MergeProvider,
) (*types.AggregatedAttestation, *types.SingleMessageAggregate, bool, error) {
	if data == nil {
		return nil, nil, false, nil
	}

	selected := selectProofs(proofs)
	if len(selected) == 0 {
		return nil, nil, false, ErrNoUsableProofs
	}

	if len(selected) == 1 {
		return attestationForProof(data, selected[0]), copyProof(selected[0]), true, nil
	}

	if merger == nil {
		return fallbackSelection(data, selected[0], ErrMergeUnavailable)
	}

	merged, err := merger.Merge(selected, data, state)
	if err != nil {
		return fallbackSelection(data, selected[0], fmt.Errorf("merge proofs: %w", err))
	}
	if !validProof(merged) {
		return fallbackSelection(data, selected[0], ErrMergeUnavailable)
	}
	return attestationForProof(data, merged), copyProof(merged), true, nil
}

func fallbackSelection(
	data *types.AttestationData,
	proof *types.SingleMessageAggregate,
	err error,
) (*types.AggregatedAttestation, *types.SingleMessageAggregate, bool, error) {
	return attestationForProof(data, proof), copyProof(proof), true, err
}

func selectProofs(proofs []*types.SingleMessageAggregate) []*types.SingleMessageAggregate {
	proofs = usableProofs(proofs)
	covered := make(map[uint64]bool)
	remaining := make([]bool, len(proofs))
	for i := range remaining {
		remaining[i] = true
	}

	var selected []*types.SingleMessageAggregate
	for {
		bestIdx := -1
		bestNew := 0
		for idx, proof := range proofs {
			if !remaining[idx] {
				continue
			}
			if hasCoveredParticipant(proof.Participants, covered) {
				continue
			}
			newCount := countNewCoverage(proof.Participants, covered)
			if newCount > bestNew {
				bestIdx = idx
				bestNew = newCount
			}
		}

		if bestIdx < 0 || bestNew == 0 {
			break
		}

		proof := proofs[bestIdx]
		selected = append(selected, proof)
		remaining[bestIdx] = false
		markParticipants(proof.Participants, covered)
	}
	return selected
}

func usableProofs(proofs []*types.SingleMessageAggregate) []*types.SingleMessageAggregate {
	usable := make([]*types.SingleMessageAggregate, 0, len(proofs))
	for _, proof := range proofs {
		if validProof(proof) {
			usable = append(usable, proof)
		}
	}
	return usable
}

func attestationForProof(data *types.AttestationData, proof *types.SingleMessageAggregate) *types.AggregatedAttestation {
	return &types.AggregatedAttestation{
		AggregationBits: copyBytes(proof.Participants),
		Data:            copyAttestationData(data),
	}
}

func countNewCoverage(bits []byte, covered map[uint64]bool) int {
	count := 0
	for vid := range types.BitlistLen(bits) {
		if types.BitlistGet(bits, vid) && !covered[vid] {
			count++
		}
	}
	return count
}

func hasCoveredParticipant(bits []byte, covered map[uint64]bool) bool {
	for vid := range types.BitlistLen(bits) {
		if types.BitlistGet(bits, vid) && covered[vid] {
			return true
		}
	}
	return false
}

func markParticipants(bits []byte, covered map[uint64]bool) {
	for vid := range types.BitlistLen(bits) {
		if types.BitlistGet(bits, vid) {
			covered[vid] = true
		}
	}
}
