package store_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// PruneOnFinalization is the only other path that clears these pools, and it
// runs on finalization alone — so a finality stall is exactly when nothing
// empties them. The head-relative sweep covers that case.
func TestPruneStaleAttestationPools(t *testing.T) {
	newStore := func() *store.ConsensusStore {
		return store.NewConsensusStore(storage.NewInMemoryBackend())
	}
	data := func(target uint64) *types.AttestationData {
		return &types.AttestationData{
			Slot:   target,
			Head:   &types.Checkpoint{},
			Target: &types.Checkpoint{Slot: target},
			Source: &types.Checkpoint{},
		}
	}
	root := func(b byte) [32]byte {
		var r [32]byte
		r[0] = b
		return r
	}

	t.Run("sweeps stale roots while finalization is stalled", func(t *testing.T) {
		s := newStore()
		head := uint64(5000)
		stale, fresh := root(1), root(2)
		s.AttestationSignatures.Insert(stale, data(100), 0, [types.SignatureSize]byte{})
		s.AttestationSignatures.Insert(fresh, data(4900), 0, [types.SignatureSize]byte{})

		store.PruneStaleAttestationPools(s, head, 50)

		snap := s.AttestationSignatures.Snapshot()
		if _, ok := snap[stale]; ok {
			t.Fatal("stale root survived the sweep")
		}
		if _, ok := snap[fresh]; !ok {
			t.Fatal("root inside the retention window was swept")
		}
	})

	t.Run("keeps roots an aggregate was built from", func(t *testing.T) {
		s := newStore()
		head := uint64(5000)
		withPayload := root(3)
		s.AttestationSignatures.Insert(withPayload, data(100), 0, [types.SignatureSize]byte{})
		participants := types.NewBitlistSSZ(1)
		types.BitlistSet(participants, 0)
		s.NewPayloads.Push(withPayload, data(100), &types.SingleMessageAggregate{
			Participants: participants,
			Proof:        []byte{1},
		})

		store.PruneStaleAttestationPools(s, head, 50)

		if _, ok := s.AttestationSignatures.Snapshot()[withPayload]; !ok {
			t.Fatal("swept the coverage a live aggregate was built from")
		}
	})

	t.Run("no-op below the finalized slot", func(t *testing.T) {
		s := newStore()
		old := root(4)
		s.AttestationSignatures.Insert(old, data(100), 0, [types.SignatureSize]byte{})

		// Finalization is healthy and past the cutoff, so PruneOnFinalization
		// owns this range and repeating it here would be wasted work.
		store.PruneStaleAttestationPools(s, 5000, 4900)

		if _, ok := s.AttestationSignatures.Snapshot()[old]; !ok {
			t.Fatal("swept below the finalized slot")
		}
	})
}
