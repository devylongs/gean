package store_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// MaxStoredBlockSlot feeds the duty gate's network-stall carve-out and is read
// up to three times a slot on the dispatch loop, so it is a high-water mark
// maintained on insert rather than a scan of every block header.
func TestMaxStoredBlockSlotTracksInserts(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())

	if got := s.MaxStoredBlockSlot(); got != 0 {
		t.Fatalf("empty store = %d, want 0", got)
	}

	s.InsertBlockHeader([32]byte{1}, &types.BlockHeader{Slot: 7})
	if got := s.MaxStoredBlockSlot(); got != 7 {
		t.Fatalf("after insert at 7 = %d, want 7", got)
	}

	// A lower slot arriving later (backfill, a sibling branch) must not lower
	// the mark — it reports the furthest the chain has been seen to reach.
	s.InsertBlockHeader([32]byte{2}, &types.BlockHeader{Slot: 3})
	if got := s.MaxStoredBlockSlot(); got != 7 {
		t.Fatalf("after insert at 3 = %d, want 7", got)
	}

	s.InsertBlockHeader([32]byte{3}, &types.BlockHeader{Slot: 12})
	if got := s.MaxStoredBlockSlot(); got != 12 {
		t.Fatalf("after insert at 12 = %d, want 12", got)
	}
}

// The mark lives in memory, so a process that restarts against an existing
// database has to recover it before the duty gate reads it. Getting this wrong
// is not a cosmetic bug: a fresh store reporting 0 makes the gate compute a
// network lag of the entire chain length, take the network-stall branch, and
// resume duties on a stale head — the dead-fork behaviour the carve-out exists
// to prevent.
//
// The pending block matters specifically. It is written by storePendingBlock
// and never imported, so it appears in TableBlockHeaders but not in the live
// chain: an index built only from imported blocks would miss it and read slot
// 100 here, ten slots behind the truth.
func TestMaxStoredBlockSlotSeedsFromDiskAfterRestart(t *testing.T) {
	backend := storage.NewInMemoryBackend()

	first := store.NewConsensusStore(backend)
	first.InsertBlockHeader([32]byte{1}, &types.BlockHeader{Slot: 100})
	first.InsertBlockHeader([32]byte{2}, &types.BlockHeader{Slot: 109})

	// Same database, new process.
	restarted := store.NewConsensusStore(backend)
	if got := restarted.MaxStoredBlockSlot(); got != 109 {
		t.Fatalf("after restart = %d, want 109 (the persisted pending block)", got)
	}

	// Seeding happens once; inserts after it still raise the mark.
	restarted.InsertBlockHeader([32]byte{3}, &types.BlockHeader{Slot: 111})
	if got := restarted.MaxStoredBlockSlot(); got != 111 {
		t.Fatalf("after post-seed insert = %d, want 111", got)
	}
}
