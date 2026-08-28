package store_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/store"
)

func TestReadMethodsHandleNilBackend(t *testing.T) {
	s := store.NewConsensusStore(nil)
	var root [32]byte

	if got := s.Time(); got != 0 {
		t.Fatalf("time=%d, want 0", got)
	}
	if got := s.Head(); got != root {
		t.Fatal("head should be zero root")
	}
	if got := s.SafeTarget(); got != root {
		t.Fatal("safe target should be zero root")
	}
	if got := s.Config(); got == nil {
		t.Fatal("config should fall back to an empty config")
	}
	if got := s.LatestJustified(); got == nil {
		t.Fatal("latest justified should fall back to an empty checkpoint")
	}
	if got := s.LatestFinalized(); got == nil {
		t.Fatal("latest finalized should fall back to an empty checkpoint")
	}
	if got := s.GetBlockHeader(root); got != nil {
		t.Fatal("block header should be nil")
	}
	if got := s.GetSignedBlock(root); got != nil {
		t.Fatal("signed block should be nil")
	}
	if got := s.GetState(root); got != nil {
		t.Fatal("state should be nil")
	}
	if s.HasState(root) {
		t.Fatal("state should not exist")
	}
	if got := s.StatesCount(); got != 0 {
		t.Fatalf("states count=%d, want 0", got)
	}
	if got := s.MaxStoredBlockSlot(); got != 0 {
		t.Fatalf("max stored slot=%d, want 0", got)
	}
	if got := s.HeadSlot(); got != 0 {
		t.Fatalf("head slot=%d, want 0", got)
	}
	if got, _ := s.GetCanonicalBlocksInRange(0, 1); got != nil {
		t.Fatalf("canonical blocks=%v, want nil", got)
	}
	if roots, err := s.BlockRoots(); err == nil || roots != nil {
		t.Fatalf("BlockRoots roots=%v err=%v, want nil roots and error", roots, err)
	}
}

func TestNilStoreReadMethodsHandleNilReceiver(t *testing.T) {
	var s *store.ConsensusStore
	var root [32]byte

	if got := s.Time(); got != 0 {
		t.Fatalf("time=%d, want 0", got)
	}
	if got := s.Config(); got == nil {
		t.Fatal("config should fall back to an empty config")
	}
	if got := s.GetBlockHeader(root); got != nil {
		t.Fatal("block header should be nil")
	}
	if got := s.GetState(root); got != nil {
		t.Fatal("state should be nil")
	}
	if roots, err := s.BlockRoots(); err == nil || roots != nil {
		t.Fatalf("BlockRoots roots=%v err=%v, want nil roots and error", roots, err)
	}
}
