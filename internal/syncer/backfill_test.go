package syncer

import (
	"context"
	"fmt"
	"testing"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/types"
)

func TestSyncDriver_CheckAndBackfill_PeerNotAhead(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	peerStatus := &p2p.StatusMessage{HeadSlot: 0}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 0 {
		t.Errorf("expected 0 range calls when peer not ahead, got %d", got)
	}
}

func TestSyncDriver_CheckAndBackfill_GapBelowThreshold(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	peerStatus := &p2p.StatusMessage{HeadSlot: blocksByRangeSyncThreshold}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 0 {
		t.Errorf("expected 0 range calls for gap below threshold, got %d", got)
	}
}

func TestSyncDriver_CheckAndBackfill_FetchesWhenAhead(t *testing.T) {
	n, store := makeTestSyncHarness()
	blocks := makeSyncRange(store.Head(), 1, 2)
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			blocks,
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	peerStatus := &p2p.StatusMessage{HeadSlot: 100}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 2 {
		t.Errorf("expected 2 range calls (drain + empty probe), got %d", got)
	}

	got := drainBlockCh(t, n, 2, 100*time.Millisecond)
	if len(got) != 2 {
		t.Fatalf("expected 2 blocks fed to engine, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_LoopsUntilCaughtUp(t *testing.T) {
	n, store := makeTestSyncHarness()
	blocks := makeSyncRange(store.Head(), 1, 2, 3, 4)
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			blocks[:2],
			blocks[2:],
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	peerStatus := &p2p.StatusMessage{HeadSlot: 100}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 3 {
		t.Errorf("expected 3 range calls (2 batches + empty probe), got %d", got)
	}

	got := drainBlockCh(t, n, 4, 100*time.Millisecond)
	if len(got) != 4 {
		t.Fatalf("expected 4 blocks fed to engine, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_CapsRangeCount(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: types.MaxRequestBlocks + 100})

	mock.mu.Lock()
	got := mock.lastRangeCount
	mock.mu.Unlock()
	if got != types.MaxRequestBlocks {
		t.Fatalf("range count=%d, want %d", got, types.MaxRequestBlocks)
	}
}

func TestSyncDriver_CheckAndBackfill_StopsOnMalformedRangeBlock(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			{nil},
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})

	if got := drainBlockCh(t, n, 1, 10*time.Millisecond); len(got) != 0 {
		t.Fatalf("malformed block should not reach node, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_RejectsBlockBeforeRequestedSlot(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			{{Block: &types.Block{Slot: 0, Body: &types.BlockBody{}}}},
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})

	if got := drainBlockCh(t, n, 1, 10*time.Millisecond); len(got) != 0 {
		t.Fatalf("old range block should not reach node, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_RejectsNonMonotonicRange(t *testing.T) {
	n, store := makeTestSyncHarness()
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			{
				{Block: &types.Block{Slot: 1, Body: &types.BlockBody{}}},
				{Block: &types.Block{Slot: 1, Body: &types.BlockBody{}}},
			},
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})

	if got := drainBlockCh(t, n, 1, 10*time.Millisecond); len(got) != 0 {
		t.Fatalf("non-monotonic range should not partially reach node, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_RejectsDisconnectedRange(t *testing.T) {
	n, store := makeTestSyncHarness()
	blocks := makeSyncRange(store.Head(), 1)
	badBlock, _ := makeSyncBlock(2, [32]byte{0xff})
	blocks = append(blocks, badBlock)
	mock := &mockSyncP2P{rangeBatches: [][]*types.SignedBlock{blocks}}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})

	if got := drainBlockCh(t, n, 1, 10*time.Millisecond); len(got) != 0 {
		t.Fatalf("disconnected range should not partially reach node, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_FallsBackToRoot(t *testing.T) {
	n, store := makeTestSyncHarness()
	rootBlock, _ := makeSyncBlock(100, types.ZeroRoot)
	mock := &mockSyncP2P{
		rangeErr: fmt.Errorf("simulated range fetch failure"),
		rootBlocks: []*types.SignedBlock{
			rootBlock,
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	var headRoot [32]byte
	headRoot[0] = 0xAB
	peerStatus := &p2p.StatusMessage{HeadSlot: 100, HeadRoot: headRoot}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)

	if got := mock.rangeCalls.Load(); got != 1 {
		t.Errorf("expected 1 range call (failed), got %d", got)
	}
	if got := mock.rootCalls.Load(); got != 1 {
		t.Errorf("expected 1 root fallback call, got %d", got)
	}

	got := drainBlockCh(t, n, 1, 100*time.Millisecond)
	if len(got) != 1 {
		t.Fatalf("expected 1 block from root fallback, got %d", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_FallbackSkipsMalformedBlocks(t *testing.T) {
	n, store := makeTestSyncHarness()
	rootBlock, _ := makeSyncBlock(100, types.ZeroRoot)
	mock := &mockSyncP2P{
		rangeErr: fmt.Errorf("simulated range fetch failure"),
		rootBlocks: []*types.SignedBlock{
			nil,
			{},
			{Block: &types.Block{Slot: 99}},
			rootBlock,
		},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})

	got := drainBlockCh(t, n, 1, 100*time.Millisecond)
	if len(got) != 1 || got[0].Block.Slot != 100 {
		t.Fatalf("fallback forwarded %d blocks, want only valid slot 100", len(got))
	}
}

func TestSyncDriver_CheckAndBackfill_PerPeerDedup(t *testing.T) {
	n, store := makeTestSyncHarness()
	rangeBlock := make(chan struct{})
	blocks := makeSyncRange(store.Head(), 1)
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{
			blocks,
		},
		rangeBlock: rangeBlock,
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	peerStatus := &p2p.StatusMessage{HeadSlot: 100}
	peerID := libp2ppeer.ID("p1")

	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		sd.checkAndBackfill(context.Background(), peerID, peerStatus)
	}()

	for mock.rangeCalls.Load() < 1 {
		time.Sleep(time.Millisecond)
	}

	sd.checkAndBackfill(context.Background(), peerID, peerStatus)
	if got := mock.rangeCalls.Load(); got != 1 {
		t.Errorf("expected 1 range call after dedup, got %d", got)
	}

	close(rangeBlock)
	<-done1
	_ = drainBlockCh(t, n, 1, 100*time.Millisecond)
}

// A full engine channel must stall the backfill, not drop blocks: dropped
// blocks are never re-requested (the range cursor advances past them), which
// permanently disconnects the chain.
func TestSyncDriver_CheckAndBackfill_BackpressuresInsteadOfDropping(t *testing.T) {
	n, store := makeTestSyncHarness()
	n.BlockCh = make(chan *types.SignedBlock, 1)
	blocks := makeSyncRange(store.Head(), 1, 2, 3, 4)
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{blocks},
	}
	sd := NewSyncDriver(context.Background(), n, store, mock)

	done := make(chan struct{})
	go func() {
		defer close(done)
		sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})
	}()

	got := drainBlockCh(t, n, 4, time.Second)
	if len(got) != 4 {
		t.Fatalf("expected all 4 blocks delivered despite capacity-1 channel, got %d", len(got))
	}
	<-done
}

// Cancellation while delivery is stalled must abort the backfill cleanly
// rather than advance to the next range.
func TestSyncDriver_CheckAndBackfill_CancelAbortsStalledDelivery(t *testing.T) {
	n, store := makeTestSyncHarness()
	n.BlockCh = make(chan *types.SignedBlock, 1)
	blocks := makeSyncRange(store.Head(), 1, 2, 3)
	mock := &mockSyncP2P{
		rangeBatches: [][]*types.SignedBlock{blocks, blocks},
	}
	ctx, cancel := context.WithCancel(context.Background())
	sd := NewSyncDriver(ctx, n, store, mock)

	done := make(chan struct{})
	go func() {
		defer close(done)
		sd.checkAndBackfill(ctx, libp2ppeer.ID("p1"), &p2p.StatusMessage{HeadSlot: 100})
	}()

	// One block fits the channel; the feed is now stalled on the second.
	_ = drainBlockCh(t, n, 1, time.Second)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("backfill did not abort after cancellation")
	}
	if got := mock.rangeCalls.Load(); got != 1 {
		t.Errorf("expected no further range fetches after aborted delivery, got %d calls", got)
	}
}
