package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func newTestStoreForBlocks() *store.ConsensusStore {
	return store.NewConsensusStore(storage.NewInMemoryBackend())
}

// emptyChainConfig is needed because the SignedBlock SSZ schema reaches through
// internal/types/block.go down to fields that need to round-trip.
func makeSignedBlockForTest(slot, proposer uint64) *types.SignedBlock {
	return &types.SignedBlock{
		Block: &types.Block{
			Slot:          slot,
			ProposerIndex: proposer,
			Body:          &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{},
	}
}

func TestFinalizedBlockHandler_NotFound_WhenNoFinalized(t *testing.T) {
	s := newTestStoreForBlocks()
	// LatestFinalized().Root == ZeroRoot when nothing has been set.

	rec := httptest.NewRecorder()
	FinalizedBlockHandler(s)(rec, httptest.NewRequest("GET", "/lean/v0/blocks/finalized", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}

func TestFinalizedBlockHandler_NotFound_WhenSignedBlockMissing(t *testing.T) {
	s := newTestStoreForBlocks()
	// Set finalized to a non-zero root but never store a SignedBlock for it.
	var root [32]byte
	root[0] = 0xab
	s.SetLatestFinalized(&types.Checkpoint{Root: root, Slot: 5})

	rec := httptest.NewRecorder()
	FinalizedBlockHandler(s)(rec, httptest.NewRequest("GET", "/lean/v0/blocks/finalized", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}

func TestFinalizedBlockHandler_ReturnsSSZ_WhenPresent(t *testing.T) {
	s := newTestStoreForBlocks()

	signed := makeSignedBlockForTest(7, 3)
	root, err := signed.Block.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash_tree_root: %v", err)
	}
	s.StorePendingBlock(root, signed)
	s.SetLatestFinalized(&types.Checkpoint{Root: root, Slot: 7})

	rec := httptest.NewRecorder()
	FinalizedBlockHandler(s)(rec, httptest.NewRequest("GET", "/lean/v0/blocks/finalized", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type=%q, want application/octet-stream", got)
	}

	want, err := signed.MarshalSSZ()
	if err != nil {
		t.Fatalf("expected MarshalSSZ: %v", err)
	}
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatalf("body mismatch:\n got len=%d\n want len=%d", rec.Body.Len(), len(want))
	}

	// Sanity: the served bytes deserialize back into the same block via SSZ.
	roundtrip := &types.SignedBlock{}
	if err := roundtrip.UnmarshalSSZ(rec.Body.Bytes()); err != nil {
		t.Fatalf("served bytes do not roundtrip: %v", err)
	}
	if roundtrip.Block.Slot != signed.Block.Slot || roundtrip.Block.ProposerIndex != signed.Block.ProposerIndex {
		t.Fatalf("roundtrip mismatch: slot=%d proposer=%d, want slot=%d proposer=%d",
			roundtrip.Block.Slot, roundtrip.Block.ProposerIndex,
			signed.Block.Slot, signed.Block.ProposerIndex)
	}
}
