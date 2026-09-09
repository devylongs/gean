package aggregation

import (
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func TestAggregationMessageRejectsNilData(t *testing.T) {
	if _, _, err := aggregationMessage(nil); err == nil {
		t.Fatal("expected nil attestation data error")
	}
}

func TestAggregationMessageRejectsSlotOverflow(t *testing.T) {
	data := &types.AttestationData{
		Slot:   uint64(^uint32(0)) + 1,
		Head:   &types.Checkpoint{},
		Target: &types.Checkpoint{},
		Source: &types.Checkpoint{},
	}

	if _, _, err := aggregationMessage(data); err == nil {
		t.Fatal("expected slot overflow error")
	}
}

func TestAggregationMessageBuildsRootAndSlot(t *testing.T) {
	data := &types.AttestationData{
		Slot:   12,
		Head:   &types.Checkpoint{Slot: 12},
		Target: &types.Checkpoint{Slot: 10},
		Source: &types.Checkpoint{Slot: 8},
	}
	wantRoot, err := data.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash data: %v", err)
	}

	root, slot, err := aggregationMessage(data)
	if err != nil {
		t.Fatalf("aggregation message: %v", err)
	}
	if root != wantRoot || slot != 12 {
		t.Fatalf("root=%x slot=%d, want %x/12", root, slot, wantRoot)
	}
}

func aggregateTestSnapshot(slots ...uint64) *Snapshot {
	snap := &Snapshot{
		// SnapshotInputs never yields a nil head state; signer resolution reads
		// its validator registry.
		headState:    &types.State{LatestFinalized: &types.Checkpoint{Slot: 0}},
		attSigs:      make(map[[32]byte]*store.AttestationDataEntry),
		newEntries:   make(map[[32]byte]*store.PayloadEntry),
		knownEntries: make(map[[32]byte]*store.PayloadEntry),
	}
	for i, slot := range slots {
		var dr [32]byte
		dr[0] = byte(i + 1)
		snap.attSigs[dr] = &store.AttestationDataEntry{
			Data: &types.AttestationData{
				Slot:   slot,
				Head:   &types.Checkpoint{},
				Target: &types.Checkpoint{Slot: slot},
				Source: &types.Checkpoint{},
			},
		}
	}
	return snap
}

func TestAggregateFromSnapshotExpiredDeadlineReportsTruncation(t *testing.T) {
	snap := aggregateTestSnapshot(5)
	cache := xmss.NewPubKeyCache()

	aggs, payloads, deletes, truncated, _ := aggregateFromSnapshot(snap, cache, time.Now().Add(-time.Second), MaxGroupsPerSession, shadow.Rates{}, newUnitCostEstimator())

	if !truncated {
		t.Fatal("expected truncation with expired deadline")
	}
	if len(aggs) != 0 || len(payloads) != 0 || len(deletes) != 0 {
		t.Fatalf("expected no results, got aggs=%d payloads=%d deletes=%d", len(aggs), len(payloads), len(deletes))
	}
}

func TestAggregateFromSnapshotZeroDeadlineProcessesAll(t *testing.T) {
	snap := aggregateTestSnapshot(5)

	_, _, _, truncated, _ := aggregateFromSnapshot(snap, xmss.NewPubKeyCache(), time.Time{}, MaxGroupsPerSession, shadow.Rates{}, newUnitCostEstimator())

	if truncated {
		t.Fatal("zero deadline must never truncate")
	}
}

// The cost of a proof is the proof, not what it covers: two-signature groups
// measured 2.0-5.2s on a 16-core host. Dividing that by the signature count is
// what previously concluded a signature costs seconds and pinned every later
// group at the two-signature floor.
func TestUnitCostEstimatorLearnsFixedCostPerProof(t *testing.T) {
	e := newUnitCostEstimator()

	for range 10 {
		e.observeGroup(4*time.Second, 0)
	}

	if got := e.nextGroupDuration(); got < 3800*time.Millisecond || got > 4200*time.Millisecond {
		t.Fatalf("nextGroupDuration=%v, want about 4s (the whole proof, not a share of it)", got)
	}
}

// Children are the part that does scale, so they are charged whatever the fixed
// cost does not explain.
func TestUnitCostEstimatorChargesChildrenTheResidual(t *testing.T) {
	e := newUnitCostEstimator()

	for range 10 {
		e.observeGroup(2*time.Second, 0)
	}
	for range 10 {
		e.observeGroup(5*time.Second, 1)
	}

	if got := e.childDuration(); got < 2500*time.Millisecond || got > 3500*time.Millisecond {
		t.Fatalf("childDuration=%v, want about 3s (5s group less the 2s baseline)", got)
	}
	if got := e.nextGroupDuration(); got > 2500*time.Millisecond {
		t.Fatalf("nextGroupDuration=%v, want the recursive group kept out of the fixed cost", got)
	}

	// A group cheaper than a raw-only one says nothing about its children.
	steady := e.childDuration()
	e.observeGroup(time.Millisecond, 1)
	if e.childDuration() != steady {
		t.Fatalf("under-cost group moved the child estimate: %v -> %v", steady, e.childDuration())
	}

	// With no baseline yet there is nothing to subtract, so a recursive group is
	// ignored rather than charged the whole duration.
	fresh := newUnitCostEstimator()
	before := fresh.childDuration()
	fresh.observeGroup(9*time.Second, 1)
	if fresh.childDuration() != before {
		t.Fatalf("child estimate moved without a fixed-cost baseline: %v -> %v", before, fresh.childDuration())
	}
}

// A budget stop defers every group still queued, not only the one it examined.
// Counting a single skip understated the backlog and made a session that dropped
// a long queue look like one that dropped a single group.
func TestAggregateFromSnapshotBudgetStopCountsEveryDeferredGroup(t *testing.T) {
	snap := aggregateTestSnapshot(5, 6, 7)
	cache := xmss.NewPubKeyCache()

	_, _, _, truncated, skips := aggregateFromSnapshot(snap, cache, time.Now().Add(-time.Second), MaxGroupsPerSession, shadow.Rates{}, newUnitCostEstimator())

	if !truncated {
		t.Fatal("expected truncation with expired deadline")
	}
	if got := skips[metrics.AggGroupSkipBudget]; got != 3 {
		t.Fatalf("budget skips = %d, want 3 (one per deferred group)", got)
	}
}

// This slot's votes are the only ones with a deadline: they must be aggregated
// in time to reach the next block, while backlog entries lose nothing by waiting
// a slot. With a session capped at two groups, ordering purely by target slot
// would spend both on the oldest backlog and leave the current slot unaggregated.
func TestOrderedGroupsPutsCurrentSlotFirst(t *testing.T) {
	snap := aggregateTestSnapshot(10, 11, 12)
	snap.slot = 12

	groups := orderedGroups(snap, groupSkips{})
	if len(groups) != 3 {
		t.Fatalf("groups=%d, want 3", len(groups))
	}
	if !groups[0].currentSlot || groups[0].targetSlot != 12 {
		t.Fatalf("first group targets slot %d (current=%v), want the current slot",
			groups[0].targetSlot, groups[0].currentSlot)
	}
	// Behind it, the frontier rule still holds: oldest unjustified target first.
	if groups[1].targetSlot != 10 || groups[2].targetSlot != 11 {
		t.Fatalf("backlog order = %d,%d, want ascending target 10,11",
			groups[1].targetSlot, groups[2].targetSlot)
	}
}
