package aggregation

import (
	"strings"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

// A session that drops every group must say so. Reporting produced=0 with no
// reason is indistinguishable from an idle aggregator, which is what hid a
// devnet-5 aggregator producing nothing for 355 consecutive slots.
func TestAggregateFromSnapshotCountsMissingTargetState(t *testing.T) {
	target := &types.Checkpoint{Slot: 42, Root: rootByte(9)}
	snap := &Snapshot{
		headState: &types.State{LatestFinalized: &types.Checkpoint{Slot: 0}},
		attSigs: map[[32]byte]*store.AttestationDataEntry{
			rootByte(1): {Data: &types.AttestationData{Slot: 42, Target: target}},
			rootByte(2): {Data: &types.AttestationData{Slot: 43, Target: target}},
		},
		// targetStates deliberately empty: no state stored for the target root.
		targetStates: map[[32]byte]*types.State{},
	}

	aggs, _, _, _, skips := aggregateFromSnapshot(snap, xmss.NewPubKeyCache(), time.Time{}, shadow.Rates{}, newUnitCostEstimator())

	if len(aggs) != 0 {
		t.Fatalf("aggregates=%d, want 0 when no target state is stored", len(aggs))
	}
	if got := skips[metrics.AggGroupSkipMissingTargetState]; got != 2 {
		t.Errorf("missing_target_state skips=%d, want 2 — the drop must be counted, not silent", got)
	}
	if skips.total() != 2 {
		t.Errorf("total skips=%d, want 2", skips.total())
	}
}

func TestOrderedGroupsCountsJustifiedSkips(t *testing.T) {
	const finalized = uint64(100)
	const justifiedTarget = uint64(105)
	const openTarget = uint64(106)

	justifiedSlots := types.BitlistExtend(nil, 10)
	types.BitlistSet(justifiedSlots, justifiedTarget-finalized-1)
	snap := &Snapshot{
		headState: &types.State{
			LatestFinalized: &types.Checkpoint{Slot: finalized},
			JustifiedSlots:  justifiedSlots,
		},
		attSigs: map[[32]byte]*store.AttestationDataEntry{
			rootByte(1): {Data: &types.AttestationData{Slot: justifiedTarget, Target: &types.Checkpoint{Slot: justifiedTarget}}},
			rootByte(2): {Data: &types.AttestationData{Slot: openTarget, Target: &types.Checkpoint{Slot: openTarget}}},
		},
	}

	skips := groupSkips{}
	ordered := orderedGroups(snap, skips)

	if len(ordered) != 1 {
		t.Fatalf("groups=%d, want 1", len(ordered))
	}
	if got := skips[metrics.AggGroupSkipTargetJustified]; got != 1 {
		t.Errorf("target_justified skips=%d, want 1", got)
	}
}

// The summary is what reaches the operator's log line, so it must name every
// non-zero reason and stay stable across runs despite map iteration order.
func TestGroupSkipsSummary(t *testing.T) {
	if got := (groupSkips{}).summary(); got != "" {
		t.Errorf("empty summary=%q, want \"\"", got)
	}

	skips := groupSkips{
		metrics.AggGroupSkipTooFewSigners:      2,
		metrics.AggGroupSkipMissingTargetState: 5,
	}
	got := skips.summary()

	for _, want := range []string{"missing_target_state=5", "too_few_signers=2"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q missing %q", got, want)
		}
	}
	for range 5 {
		if again := skips.summary(); again != got {
			t.Fatalf("summary unstable: %q then %q", got, again)
		}
	}
}
