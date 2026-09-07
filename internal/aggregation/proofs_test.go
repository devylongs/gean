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

// Raw-first selection removes most recursion, but a group with no local raw
// coverage can still reach for several children. The cap bounds what one group
// may spend, and it must hold across the new-payload and known-payload passes
// that share the same children slice.
func TestSelectChildProofsCapsChildrenPerGroup(t *testing.T) {
	state := &types.State{Validators: []*types.Validator{
		{Index: 0}, {Index: 1}, {Index: 2}, {Index: 3}, {Index: 4}, {Index: 5},
	}}
	proof := func(ids ...uint64) *types.SingleMessageAggregate {
		return &types.SingleMessageAggregate{Participants: types.BitlistFromIndices(ids), Proof: []byte{1}}
	}
	newEntry := &store.PayloadEntry{Proofs: []*types.SingleMessageAggregate{proof(0, 1), proof(2, 3)}}
	knownEntry := &store.PayloadEntry{Proofs: []*types.SingleMessageAggregate{proof(4, 5)}}

	cache := xmss.NewPubKeyCache()
	defer cache.Close()

	var children []xmss.ChildProof
	covered := map[uint64]bool{}
	remaining := 100

	selectChildProofs(newEntry, state, &children, covered, cache, &remaining)
	selectChildProofs(knownEntry, state, &children, covered, cache, &remaining)

	if len(children) != maxChildProofsPerGroup {
		t.Fatalf("children=%d, want %d", len(children), maxChildProofsPerGroup)
	}
	// The third proof's validators stay uncovered: the cap dropped it rather
	// than the pass running out of coverage to add.
	if covered[4] || covered[5] {
		t.Fatal("cap did not stop the known-payload pass")
	}
}
