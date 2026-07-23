package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestOnGossipAggregatedAttestationRejectsMissingProof(t *testing.T) {
	e := makeTestEngine()
	head := e.Store.Head()
	attData := &types.AttestationData{
		Slot:   0,
		Source: &types.Checkpoint{Root: head, Slot: 0},
		Target: &types.Checkpoint{Root: head, Slot: 0},
		Head:   &types.Checkpoint{Root: head, Slot: 0},
	}

	e.onGossipAggregatedAttestation(&types.SignedAggregatedAttestation{
		Data:  attData,
		Proof: nil,
	})
	if e.Store.NewPayloads.Len() != 0 {
		t.Fatalf("payloads=%d, want 0 after nil proof", e.Store.NewPayloads.Len())
	}

	e.onGossipAggregatedAttestation(&types.SignedAggregatedAttestation{
		Data: attData,
		Proof: &types.SingleMessageAggregate{
			Participants: types.NewBitlistSSZ(0),
			Proof:        nil,
		},
	})
	if e.Store.NewPayloads.Len() != 0 {
		t.Fatalf("payloads=%d, want 0 after empty proof", e.Store.NewPayloads.Len())
	}
}
