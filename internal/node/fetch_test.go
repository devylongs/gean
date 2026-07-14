package node

import (
	"testing"

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
