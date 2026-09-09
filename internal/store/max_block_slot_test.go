package store_test

import (
	"errors"
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

// failOncePartialBackend truncates the first block-header scan part-way through
// and then behaves normally, so the same store can be seeded twice.
//
// The partial scan is the dangerous shape: it returns a plausible, lower slot
// rather than an obvious zero, so a caller that mistakes "iteration stopped" for
// "end of table" caches an understated watermark that looks entirely reasonable.
type failOncePartialBackend struct {
	storage.Backend
	entriesBeforeFailure int
	failed               bool
}

func (b *failOncePartialBackend) BeginRead() (storage.ReadView, error) {
	rv, err := b.Backend.BeginRead()
	if err != nil {
		return nil, err
	}
	return &failOnceView{ReadView: rv, backend: b}, nil
}

type failOnceView struct {
	storage.ReadView
	backend *failOncePartialBackend
}

func (v *failOnceView) PrefixIterator(table storage.Table, prefix []byte) (storage.Iterator, error) {
	it, err := v.ReadView.PrefixIterator(table, prefix)
	if err != nil || table != storage.TableBlockHeaders || v.backend.failed {
		return it, err
	}
	v.backend.failed = true
	return &partialIterator{Iterator: it, remaining: v.backend.entriesBeforeFailure}, nil
}

// partialIterator yields a few real entries and then reports the failure the way
// a storage backend does: Next goes false, exactly as at the end of the range,
// with the reason available only from Err.
type partialIterator struct {
	storage.Iterator
	remaining int
}

func (it *partialIterator) Next() bool {
	if it.remaining <= 0 {
		return false
	}
	it.remaining--
	return it.Iterator.Next()
}

func (it *partialIterator) Err() error {
	if it.remaining <= 0 {
		return errors.New("iteration failed part-way")
	}
	return it.Iterator.Err()
}

// A seeding scan that fails must not latch, and the *same* store must recover on
// a later attempt.
//
// Caching a partial answer is worse than retrying: an understated watermark
// inflates the duty gate's computed network lag, which can trip the
// network-stall carve-out and let the node resume duties on a stale head - the
// opposite of what the gate is for.
func TestMaxStoredBlockSlotRetriesAfterAFailedSeed(t *testing.T) {
	inner := storage.NewInMemoryBackend()

	// Root order drives iteration order, so the truncated scan sees slot 5 and
	// stops before slot 109. A latched failure would leave the mark at neither
	// the truth nor an obvious zero.
	seed := store.NewConsensusStore(inner)
	seed.InsertBlockHeader([32]byte{0x01}, &types.BlockHeader{Slot: 5})
	seed.InsertBlockHeader([32]byte{0x02}, &types.BlockHeader{Slot: 109})

	backend := &failOncePartialBackend{Backend: inner, entriesBeforeFailure: 1}
	s := store.NewConsensusStore(backend)

	if err := s.SeedMaxStoredBlockSlot(); err == nil {
		t.Fatal("truncated scan reported success")
	}
	if got := s.MaxStoredBlockSlot(); got != 109 {
		// MaxStoredBlockSlot retries internally, so by now the second attempt
		// has run and must have found the real maximum. A latched failure would
		// return 0 or the partial 5.
		t.Fatalf("after retry = %d, want 109", got)
	}
}

// The same thing without going through MaxStoredBlockSlot, so the retry is
// attributable to SeedMaxStoredBlockSlot itself rather than to a caller.
func TestSeedMaxStoredBlockSlotIsRetryableOnTheSameStore(t *testing.T) {
	inner := storage.NewInMemoryBackend()
	seed := store.NewConsensusStore(inner)
	seed.InsertBlockHeader([32]byte{0x01}, &types.BlockHeader{Slot: 5})
	seed.InsertBlockHeader([32]byte{0x02}, &types.BlockHeader{Slot: 109})

	s := store.NewConsensusStore(&failOncePartialBackend{Backend: inner, entriesBeforeFailure: 1})

	if err := s.SeedMaxStoredBlockSlot(); err == nil {
		t.Fatal("first seed reported success on a truncated scan")
	}
	if err := s.SeedMaxStoredBlockSlot(); err != nil {
		t.Fatalf("second seed on the same store failed: %v", err)
	}
	if got := s.MaxStoredBlockSlot(); got != 109 {
		t.Fatalf("after successful reseed = %d, want 109", got)
	}
}
