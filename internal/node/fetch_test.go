package node

import (
	"context"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/types"
)

// A missing parent is re-derived on every child that references it, so without dedup the
// same root floods FetchRootCh and starves the fetch. Each root must be queued at most
// once until its block is received or its fetch is exhausted.
func TestQueueMissingBlockFetchDedupes(t *testing.T) {
	e := makeTestEngine()
	e.P2P = &p2p.Host{} // non-nil so the fetch path runs; queueMissingBlockFetch calls no P2P method

	var root [32]byte
	root[0] = 0x77

	for i := 0; i < 10; i++ {
		e.queueMissingBlockFetch(root)
	}
	if got := len(e.FetchRootCh); got != 1 {
		t.Fatalf("FetchRootCh has %d requests after 10 queues, want 1 (deduped)", got)
	}
	if !e.fetchInFlight[root] {
		t.Fatal("queued root should be marked in-flight")
	}

	// Exhausting the fetch clears the marker so a later gap can re-request the root.
	e.onFailedRoot(root)
	if e.fetchInFlight[root] {
		t.Fatal("onFailedRoot should clear the in-flight marker")
	}
	<-e.FetchRootCh // drain the first request so the channel has room
	e.queueMissingBlockFetch(root)
	if got := len(e.FetchRootCh); got != 1 {
		t.Fatalf("re-queue after exhaustion: FetchRootCh has %d, want 1", got)
	}
}

// A dropped failed-root notification strands its in-flight marker set forever, so the
// root is never re-requested and the gap never closes. Delivery must wait for the
// dispatch loop rather than drop.
func TestNotifyFailedRootsBlocksInsteadOfDropping(t *testing.T) {
	e := makeTestEngine()
	for len(e.FailedRootCh) < cap(e.FailedRootCh) {
		e.FailedRootCh <- [32]byte{}
	}

	var root [32]byte
	root[0] = 0xAB

	done := make(chan struct{})
	go func() {
		e.notifyFailedRoots(context.Background(), [][32]byte{root})
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("notifyFailedRoots returned with a full channel; the notification was dropped")
	case <-time.After(50 * time.Millisecond):
	}

	<-e.FailedRootCh // dispatch loop makes room
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("notifyFailedRoots did not deliver after the channel drained")
	}

	// Drain the backlog to reach the root that was waiting.
	var delivered bool
	for len(e.FailedRootCh) > 0 {
		if <-e.FailedRootCh == root {
			delivered = true
		}
	}
	if !delivered {
		t.Fatal("failed root was never delivered to the dispatch loop")
	}
}

// Shutdown must not wedge the fetch batcher on a dispatch loop that has already stopped.
func TestNotifyFailedRootsAbortsOnContextCancel(t *testing.T) {
	e := makeTestEngine()
	for len(e.FailedRootCh) < cap(e.FailedRootCh) {
		e.FailedRootCh <- [32]byte{}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.notifyFailedRoots(ctx, [][32]byte{{0xCD}})
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("notifyFailedRoots ignored context cancellation")
	}
}

// Receiving the block for a queued root clears its in-flight marker regardless of whether
// it imports or gets buffered — we hold it now, so there is nothing left to fetch.
func TestProcessOneBlockClearsFetchMarker(t *testing.T) {
	e := makeTestEngine()

	var parentRoot [32]byte
	parentRoot[0] = 0x99 // unknown parent → the block will be buffered, not imported
	signed := &types.SignedBlock{
		Block: &types.Block{Slot: 5, ParentRoot: parentRoot, Body: &types.BlockBody{}},
		Proof: &types.MultiMessageAggregate{},
	}
	blockRoot, err := signed.Block.HashTreeRoot()
	if err != nil {
		t.Fatalf("block root: %v", err)
	}
	e.fetchInFlight[blockRoot] = true

	var queue []*types.SignedBlock
	e.processOneBlock(signed, &queue)

	if e.fetchInFlight[blockRoot] {
		t.Fatal("receiving the block should clear its fetch marker")
	}
}
