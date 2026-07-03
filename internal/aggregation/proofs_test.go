package aggregation

import (
	"testing"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func TestSelectChildProofsSkipsOutOfRangeParticipant(t *testing.T) {
	entry := &store.PayloadEntry{
		Proofs: []*types.SingleMessageAggregate{{
			Participants: types.BitlistFromIndices([]uint64{2}),
			Proof:        []byte{1},
		}},
	}
	state := &types.State{
		Validators: []*types.Validator{{Index: 0}},
	}

	var children []xmss.ChildProof
	covered := make(map[uint64]bool)
	remaining := 8
	selectChildProofs(entry, state, &children, covered, xmss.NewPubKeyCache(), &remaining)

	if len(children) != 0 {
		t.Fatalf("children=%d, want 0", len(children))
	}
	if covered[2] {
		t.Fatal("out-of-range validator marked covered")
	}
}

func TestSelectChildProofsStopsAtBudget(t *testing.T) {
	entry := &store.PayloadEntry{
		Proofs: []*types.SingleMessageAggregate{{
			Participants: types.BitlistFromIndices([]uint64{0}),
			Proof:        []byte{1},
		}},
	}
	state := &types.State{
		Validators: []*types.Validator{{Index: 0}},
	}

	var children []xmss.ChildProof
	covered := make(map[uint64]bool)
	remaining := 0
	selectChildProofs(entry, state, &children, covered, xmss.NewPubKeyCache(), &remaining)

	if len(children) != 0 {
		t.Fatalf("children=%d, want 0 (budget exhausted)", len(children))
	}
	if len(covered) != 0 {
		t.Fatal("exhausted budget must not process any proof")
	}
}
