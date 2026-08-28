package syncer

import (
	"context"
	"testing"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/types"
)

// setHeadAtSlot anchors the store's head at slot so HeadSlot() reflects it.
func setHeadAtSlot(t *testing.T, sd *SyncDriver, slot uint64) {
	t.Helper()
	head := &types.BlockHeader{Slot: slot}
	root := mustHeaderRoot(head)
	sd.store.SetHead(root)
	sd.store.InsertBlockHeader(root, head)
}

func TestSyncDriver_BeyondHistoryHorizon(t *testing.T) {
	tests := []struct {
		name     string
		ourHead  uint64
		peerHead uint64
		want     bool
	}{
		{"peer behind us", 5000, 100, false},
		{"peer level", 5000, 5000, false},
		{"gap inside window", 5000, 5000 + types.MinSlotsForBlockRequests, false},
		{"gap exactly at window edge", 100, 100 + types.MinSlotsForBlockRequests, false},
		{"gap one past the window", 100, 101 + types.MinSlotsForBlockRequests, true},
		{"devnet-5 observed gap", 97861, 103265, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, st := makeTestSyncHarness()
			sd := NewSyncDriver(context.Background(), n, st, &mockSyncP2P{})
			setHeadAtSlot(t, sd, tt.ourHead)

			got := sd.beyondHistoryHorizon(&p2p.StatusMessage{HeadSlot: tt.peerHead})
			if got != tt.want {
				t.Errorf("beyondHistoryHorizon(our=%d peer=%d) = %v, want %v",
					tt.ourHead, tt.peerHead, got, tt.want)
			}
		})
	}
}

func TestSyncDriver_BeyondHistoryHorizon_NilInputs(t *testing.T) {
	n, st := makeTestSyncHarness()
	sd := NewSyncDriver(context.Background(), n, st, &mockSyncP2P{})

	if sd.beyondHistoryHorizon(nil) {
		t.Error("nil peer status must not report beyond-horizon")
	}
	var nilDriver *SyncDriver
	if nilDriver.beyondHistoryHorizon(&p2p.StatusMessage{HeadSlot: 1 << 20}) {
		t.Error("nil driver must not report beyond-horizon")
	}
}

// A gap past the window can only be answered with RESOURCE_UNAVAILABLE, so the
// driver must stop asking rather than reissue an unanswerable request every poll.
func TestSyncDriver_CheckAndBackfill_SkipsRequestBeyondHorizon(t *testing.T) {
	n, st := makeTestSyncHarness()
	mock := &mockSyncP2P{}
	sd := NewSyncDriver(context.Background(), n, st, mock)
	setHeadAtSlot(t, sd, 97861)

	peerStatus := &p2p.StatusMessage{HeadSlot: 103265, FinalizedSlot: 92908}
	for i := 0; i < 3; i++ {
		sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), peerStatus)
	}

	if got := mock.rangeCalls.Load(); got != 0 {
		t.Errorf("expected no range requests beyond the block-request window, got %d", got)
	}
	if got := mock.rootCalls.Load(); got != 0 {
		t.Errorf("expected no by-root fallback beyond the window, got %d", got)
	}
}

// The report latches so it states the condition once, and unlatches once the node
// is back inside the window so a later episode is reported again.
func TestSyncDriver_BeyondHorizonReportLatch(t *testing.T) {
	n, st := makeTestSyncHarness()
	sd := NewSyncDriver(context.Background(), n, st, &mockSyncP2P{})
	setHeadAtSlot(t, sd, 100)

	far := &p2p.StatusMessage{HeadSlot: 100 + types.MinSlotsForBlockRequests + 1}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p1"), far)
	if !sd.horizonReported {
		t.Fatal("expected beyond-horizon condition to latch")
	}

	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p2"), far)
	if !sd.horizonReported {
		t.Fatal("latch must stay set while the condition holds")
	}

	// Back inside the window: the gap is servable again, so the latch clears.
	near := &p2p.StatusMessage{HeadSlot: 100 + blocksByRangeSyncThreshold + 1}
	sd.checkAndBackfill(context.Background(), libp2ppeer.ID("p3"), near)
	if sd.horizonReported {
		t.Fatal("expected latch to clear once the gap is inside the window")
	}
}
