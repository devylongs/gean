package node

import (
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/types"
)

func blockAtSlot(slot uint64) *types.SignedBlock {
	return &types.SignedBlock{Block: &types.Block{Slot: slot, Body: &types.BlockBody{}}}
}

// The drain must import only what was queued on entry. Sync delivery blocks on
// BlockCh, so a producer refills every slot the drain frees; draining until the
// channel reads empty hands the tick loop unbounded work, and the store clock —
// already advanced for this tick — then sits stale for the whole drain, which is
// what makes correctly-timed blocks fail the future-horizon check.
//
// The channel starts full with a producer blocked on it, so the depth at entry is
// exactly the capacity no matter how the two goroutines interleave.
func TestDrainPendingBlocks_ImportsOnlyTheQueueDepthOnEntry(t *testing.T) {
	const capacity = 8

	e := &Engine{
		Store:   makeTestStore(),
		BlockCh: make(chan *types.SignedBlock, capacity),
	}
	for i := 0; i < capacity; i++ {
		e.BlockCh <- blockAtSlot(uint64(i))
	}

	stop := make(chan struct{})
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		for {
			select {
			case <-stop:
				return
			case e.BlockCh <- blockAtSlot(999):
			}
		}
	}()
	defer func() {
		close(stop)
		<-producerDone
	}()

	type result struct{ drained int }
	got := make(chan result, 1)
	go func() { got <- result{e.drainPendingBlocks()} }()

	select {
	case r := <-got:
		if r.drained != capacity {
			t.Errorf("drained %d blocks, want %d (the depth at entry) — a refilling producer extended the drain",
				r.drained, capacity)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("drainPendingBlocks never returned: a refilling producer extended it without bound")
	}
}

func TestDrainPendingBlocks_EmptyChannelReturns(t *testing.T) {
	e := &Engine{
		Store:   makeTestStore(),
		BlockCh: make(chan *types.SignedBlock, 4),
	}

	done := make(chan struct{})
	go func() {
		if drained := e.drainPendingBlocks(); drained != 0 {
			t.Errorf("drained %d blocks from an empty channel, want 0", drained)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("drainPendingBlocks blocked on an empty channel")
	}
}

// Bounding the drain must not leave admitted blocks stranded: with no concurrent
// producer, everything queued on entry is imported.
func TestDrainPendingBlocks_ConsumesEveryQueuedBlock(t *testing.T) {
	e := &Engine{
		Store:   makeTestStore(),
		BlockCh: make(chan *types.SignedBlock, 16),
	}

	const queued = 8
	for i := 0; i < queued; i++ {
		e.BlockCh <- blockAtSlot(uint64(i))
	}

	if drained := e.drainPendingBlocks(); drained != queued {
		t.Errorf("drained = %d, want %d", drained, queued)
	}
	if remaining := len(e.BlockCh); remaining != 0 {
		t.Errorf("expected the queued blocks to be consumed, %d left in channel", remaining)
	}
}
