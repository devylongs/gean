package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

// Recovery is best-effort; the slot's aggregate is duty work. A split takes about as
// long as aggregation waits for the prover, so recovery must not start one that would
// still be running when aggregation dispatches at interval 2, nor while the aggregation
// session itself holds the prover.
func TestSplitFitsBeforeAggregation(t *testing.T) {
	e := makeTestEngine()
	genesisMs := e.Store.Config().GenesisTime * 1000

	const interval = types.MillisecondsPerInterval

	tests := []struct {
		name     string
		intoSlot uint64
		want     bool
	}{
		{"slot start, two intervals of room", 0, true},
		{"start of interval 1, exactly one interval of room", interval, true},
		{"mid interval 1, split would run past dispatch", interval + 400, false},
		{"dispatch instant", 2 * interval, false},
		{"inside aggregation session", 3 * interval, false},
		{"session budget just elapsed, prover free again", 4 * interval, true},
		{"late interval 4, next dispatch still far", 4*interval + 400, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Probe a later slot so the check is independent of the genesis boundary.
			nowMs := genesisMs + 10*types.MillisecondsPerSlot + tc.intoSlot
			if got := e.splitFitsBeforeAggregation(nowMs); got != tc.want {
				t.Fatalf("splitFitsBeforeAggregation(+%dms into slot) = %t, want %t",
					tc.intoSlot, got, tc.want)
			}
		})
	}
}

// The window check runs before the gate is acquired, and Acquire blocks for as long as
// the current holder keeps the prover. A stale pass must not authorise a split: the
// check is re-evaluated on acquisition, so a window that closed while waiting is caught.
func TestSplitFitsBeforeAggregationRejectsWindowClosedWhileWaiting(t *testing.T) {
	e := makeTestEngine()
	genesisMs := e.Store.Config().GenesisTime * 1000
	base := genesisMs + 10*types.MillisecondsPerSlot

	// Cleared at the start of the slot...
	if !e.splitFitsBeforeAggregation(base) {
		t.Fatal("split at slot start should be cleared")
	}
	// ...but a wait spanning into the dispatch window must no longer be cleared.
	if e.splitFitsBeforeAggregation(base + 2*types.MillisecondsPerInterval) {
		t.Fatal("split must not be cleared once the aggregation window has opened")
	}
}
