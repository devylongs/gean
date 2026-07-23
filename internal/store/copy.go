package store

import "github.com/geanlabs/gean/internal/types"

func copyAttestationData(data *types.AttestationData) *types.AttestationData {
	if data == nil {
		return nil
	}
	return &types.AttestationData{
		Slot:   data.Slot,
		Head:   copyCheckpoint(data.Head),
		Target: copyCheckpoint(data.Target),
		Source: copyCheckpoint(data.Source),
	}
}

func copyCheckpoint(cp *types.Checkpoint) *types.Checkpoint {
	if cp == nil {
		return nil
	}
	out := *cp
	return &out
}

func copyProof(proof *types.SingleMessageAggregate) *types.SingleMessageAggregate {
	if proof == nil {
		return nil
	}
	return &types.SingleMessageAggregate{
		Participants: copyBytes(proof.Participants),
		Proof:        copyBytes(proof.Proof),
	}
}

func copyBytes(data []byte) []byte {
	if data == nil {
		return nil
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out
}
