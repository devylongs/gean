package blockbuilder

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func mockAttestationData() *types.AttestationData {
	return &types.AttestationData{
		Slot:   10,
		Head:   &types.Checkpoint{Slot: 10},
		Target: &types.Checkpoint{Slot: 8},
		Source: &types.Checkpoint{Slot: 4},
	}
}

func mockProof(ids []uint64) *types.SingleMessageAggregate {
	return &types.SingleMessageAggregate{
		Participants: types.BitlistFromIndices(ids),
		Proof:        []byte{0xde, 0xad},
	}
}

func hashAttestationData(t *testing.T, data *types.AttestationData) [32]byte {
	t.Helper()

	root, err := data.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash attestation data: %v", err)
	}
	return root
}
