package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func aggAttWithSource(source *types.Checkpoint, headTag byte, bits []uint64) *types.AggregatedAttestation {
	return &types.AggregatedAttestation{
		AggregationBits: types.BitlistFromIndices(bits),
		Data: &types.AttestationData{
			Slot:   10,
			Head:   &types.Checkpoint{Root: [32]byte{headTag}, Slot: 9},
			Target: &types.Checkpoint{Root: [32]byte{0x0a}, Slot: 10},
			Source: source,
		},
	}
}

func noEntries() map[[32]byte]*store.PayloadEntry {
	return map[[32]byte]*store.PayloadEntry{}
}

// Only votes whose source equals the head's justified checkpoint are recoverable;
// any other source (different root, different slot, or nil) is dropped.
func TestSelectRecoveryCandidatesSourceFilter(t *testing.T) {
	root := [32]byte{0x33}
	head := &types.Checkpoint{Root: root, Slot: 3}

	attestations := []*types.AggregatedAttestation{
		aggAttWithSource(&types.Checkpoint{Root: root, Slot: 3}, 0x01, []uint64{0, 1}),           // match
		aggAttWithSource(&types.Checkpoint{Root: [32]byte{0x44}, Slot: 3}, 0x02, []uint64{0, 1}), // wrong root
		aggAttWithSource(&types.Checkpoint{Root: root, Slot: 4}, 0x03, []uint64{0, 1}),           // wrong slot
		aggAttWithSource(nil, 0x04, []uint64{0, 1}),                                              // nil source
	}

	candidates := selectRecoveryCandidates(attestations, head, noEntries(), noEntries())

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate matching head justified, got %d", len(candidates))
	}
	if *candidates[0].att.Data.Source != *head {
		t.Fatalf("candidate source %+v != head justified %+v", candidates[0].att.Data.Source, head)
	}
}

// An attestation adding no participants beyond the locally-held proofs is skipped.
func TestSelectRecoveryCandidatesSkipsFullyCovered(t *testing.T) {
	head := &types.Checkpoint{Root: [32]byte{0x33}, Slot: 3}
	att := aggAttWithSource(&types.Checkpoint{Root: [32]byte{0x33}, Slot: 3}, 0x01, []uint64{0, 1, 2})

	root, err := att.Data.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash tree root: %v", err)
	}
	known := map[[32]byte]*store.PayloadEntry{
		root: {Proofs: []*types.SingleMessageAggregate{
			{Participants: types.BitlistFromIndices([]uint64{0, 1, 2})},
		}},
	}

	candidates := selectRecoveryCandidates([]*types.AggregatedAttestation{att}, head, noEntries(), known)
	if len(candidates) != 0 {
		t.Fatalf("expected fully-covered attestation to be skipped, got %d candidates", len(candidates))
	}
}

// Candidates are ranked by new participants and capped to maxRecoverySplits to bound
// proving work.
func TestSelectRecoveryCandidatesRanksAndCaps(t *testing.T) {
	head := &types.Checkpoint{Root: [32]byte{0x33}, Slot: 3}

	// Six matching attestations with 1..6 participants each, distinct data roots via
	// the head tag so none collide in the coverage maps.
	var attestations []*types.AggregatedAttestation
	for i := 1; i <= maxRecoverySplits+2; i++ {
		bits := make([]uint64, i)
		for j := range bits {
			bits[j] = uint64(j)
		}
		attestations = append(attestations, aggAttWithSource(&types.Checkpoint{Root: [32]byte{0x33}, Slot: 3}, byte(i), bits))
	}

	candidates := selectRecoveryCandidates(attestations, head, noEntries(), noEntries())

	if len(candidates) != maxRecoverySplits {
		t.Fatalf("expected cap of %d candidates, got %d", maxRecoverySplits, len(candidates))
	}
	for i := 1; i < len(candidates); i++ {
		if candidates[i-1].newCount < candidates[i].newCount {
			t.Fatalf("candidates not ranked by new participants: %d before %d",
				candidates[i-1].newCount, candidates[i].newCount)
		}
	}
	if candidates[0].newCount != maxRecoverySplits+2 {
		t.Fatalf("expected highest newCount %d first, got %d", maxRecoverySplits+2, candidates[0].newCount)
	}
}

// Malformed attestations are dropped without panicking.
func TestSelectRecoveryCandidatesNilSafe(t *testing.T) {
	head := &types.Checkpoint{Root: [32]byte{0x33}, Slot: 3}
	attestations := []*types.AggregatedAttestation{
		nil,
		{Data: nil},
		{Data: &types.AttestationData{Source: nil}},
	}

	candidates := selectRecoveryCandidates(attestations, head, noEntries(), noEntries())
	if len(candidates) != 0 {
		t.Fatalf("expected malformed attestations to be dropped, got %d", len(candidates))
	}
}
