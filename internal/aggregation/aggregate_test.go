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

	aggs, payloads, deletes, truncated, _ := aggregateFromSnapshot(snap, cache, time.Now().Add(-time.Second), shadow.Rates{}, newUnitCostEstimator())

	if !truncated {
		t.Fatal("expected truncation with expired deadline")
	}
	if len(aggs) != 0 || len(payloads) != 0 || len(deletes) != 0 {
		t.Fatalf("expected no results, got aggs=%d payloads=%d deletes=%d", len(aggs), len(payloads), len(deletes))
	}
}

func TestAggregateFromSnapshotZeroDeadlineProcessesAll(t *testing.T) {
	snap := aggregateTestSnapshot(5)

	_, _, _, truncated, _ := aggregateFromSnapshot(snap, xmss.NewPubKeyCache(), time.Time{}, shadow.Rates{}, newUnitCostEstimator())

	if truncated {
		t.Fatal("zero deadline must never truncate")
	}
}

func TestUnitCostEstimatorMaxUnitsWithin(t *testing.T) {
	e := newUnitCostEstimator() // seed 0.1s/unit

	if got := e.maxUnitsWithin(time.Second); got != 10 {
		t.Fatalf("maxUnitsWithin(1s)=%d, want 10", got)
	}
	// Never below the spec minimum of two, even for a tiny or expired budget.
	if got := e.maxUnitsWithin(time.Millisecond); got != 2 {
		t.Fatalf("maxUnitsWithin(1ms)=%d, want 2 (floor)", got)
	}
	if got := e.maxUnitsWithin(-time.Second); got != 2 {
		t.Fatalf("maxUnitsWithin(-1s)=%d, want 2 (floor)", got)
	}
}

func TestUnitCostEstimatorObserveConverges(t *testing.T) {
	e := newUnitCostEstimator() // seed 0.1s/unit

	// A cheaper-than-seed observation must pull the estimate down, letting more
	// units fit the budget on the next pass.
	before := e.maxUnitsWithin(time.Second)
	for range 20 {
		e.observe(200*time.Millisecond, 10) // 0.02s/unit
	}
	after := e.maxUnitsWithin(time.Second)
	if after <= before {
		t.Fatalf("estimate did not converge down: before=%d after=%d units/sec", before, after)
	}

	// Degenerate inputs are ignored, not divided by.
	steady := e.perUnitSeconds
	e.observe(0, 10)
	e.observe(time.Second, 0)
	if e.perUnitSeconds != steady {
		t.Fatalf("degenerate observe mutated estimate: %v -> %v", steady, e.perUnitSeconds)
	}
}

// A budget stop defers every group still queued, not only the one it examined.
// Counting a single skip understated the backlog and made a session that dropped
// a long queue look like one that dropped a single group.
func TestAggregateFromSnapshotBudgetStopCountsEveryDeferredGroup(t *testing.T) {
	snap := aggregateTestSnapshot(5, 6, 7)
	cache := xmss.NewPubKeyCache()

	_, _, _, truncated, skips := aggregateFromSnapshot(snap, cache, time.Now().Add(-time.Second), shadow.Rates{}, newUnitCostEstimator())

	if !truncated {
		t.Fatal("expected truncation with expired deadline")
	}
	if got := skips[metrics.AggGroupSkipBudget]; got != 3 {
		t.Fatalf("budget skips = %d, want 3 (one per deferred group)", got)
	}
}
