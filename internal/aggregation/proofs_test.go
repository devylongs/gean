package aggregation

import (
	"testing"
	"time"

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
	remaining := 8 * time.Second
	selectChildProofs(entry, state, &children, covered, xmss.NewPubKeyCache(), &remaining, time.Second, 0)

	if len(children) != 0 {
		t.Fatalf("children=%d, want 0", len(children))
	}
	if covered[2] {
		t.Fatal("out-of-range validator marked covered")
	}
}

func TestSelectChildProofsAdmitsFirstChildThenPricesTheRest(t *testing.T) {
	// One raw signature already held, so the group reaches viability with its
	// first child. That child is exempt from the price regardless — validators
	// reachable only through a child proof have no raw fallback. The second is
	// charged what it costs.
	proof := func(ids ...uint64) *types.SingleMessageAggregate {
		return &types.SingleMessageAggregate{Participants: types.BitlistFromIndices(ids), Proof: []byte{1}}
	}
	state := &types.State{Validators: []*types.Validator{{Index: 0}, {Index: 1}}}

	for _, tc := range []struct {
		name      string
		remaining time.Duration
		childCost time.Duration
		want      int
	}{
		{name: "no_budget_still_admits_one", remaining: 0, childCost: 5 * time.Second, want: 1},
		{name: "budget_admits_both", remaining: 20 * time.Second, childCost: 5 * time.Second, want: 2},
		{name: "budget_short_of_second", remaining: 3 * time.Second, childCost: 5 * time.Second, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := &store.PayloadEntry{Proofs: []*types.SingleMessageAggregate{proof(0), proof(1)}}
			cache := xmss.NewPubKeyCache()
			defer cache.Close()

			var children []xmss.ChildProof
			covered := make(map[uint64]bool)
			remaining := tc.remaining
			selectChildProofs(entry, state, &children, covered, cache, &remaining, tc.childCost, 1)

			if len(children) != tc.want {
				t.Fatalf("children=%d, want %d", len(children), tc.want)
			}
		})
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
	remaining := 100 * time.Second

	selectChildProofs(newEntry, state, &children, covered, cache, &remaining, time.Second, 0)
	selectChildProofs(knownEntry, state, &children, covered, cache, &remaining, time.Second, 0)

	if len(children) != maxChildProofsPerGroup {
		t.Fatalf("children=%d, want %d", len(children), maxChildProofsPerGroup)
	}
	// The third proof's validators stay uncovered: the cap dropped it rather
	// than the pass running out of coverage to add.
	if covered[4] || covered[5] {
		t.Fatal("cap did not stop the known-payload pass")
	}
}
