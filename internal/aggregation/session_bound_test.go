package aggregation

import (
	"testing"
	"time"
)

// TestEstimatorSeedGroupDuration: before any group is observed the estimator
// reports the seed, so the very first session still attempts a group.
func TestEstimatorSeedGroupDuration(t *testing.T) {
	e := newUnitCostEstimator()
	want := time.Duration(seedPerGroupSeconds * float64(time.Second))
	if got := e.nextGroupDuration(); got != want {
		t.Fatalf("seed nextGroupDuration=%v, want %v", got, want)
	}
	// Nil receiver must not panic and must fall back to the seed.
	var nilEst *unitCostEstimator
	if got := nilEst.nextGroupDuration(); got != want {
		t.Fatalf("nil nextGroupDuration=%v, want %v", got, want)
	}
}

// TestEstimatorTracksGroupCost: the estimate follows observed per-group wall
// time via EMA, so the bound rises as proofs get more expensive with scale.
func TestEstimatorTracksGroupCost(t *testing.T) {
	e := newUnitCostEstimator()
	// First observation seeds the running value directly.
	e.observeGroup(2 * time.Second)
	if got := e.nextGroupDuration(); got < 1900*time.Millisecond || got > 2100*time.Millisecond {
		t.Fatalf("after first observe nextGroupDuration=%v, want ~2s", got)
	}
	// Repeated 2s observations converge toward 2s.
	for range 10 {
		e.observeGroup(2 * time.Second)
	}
	if got := e.nextGroupDuration(); got < 1900*time.Millisecond || got > 2100*time.Millisecond {
		t.Fatalf("converged nextGroupDuration=%v, want ~2s", got)
	}
}

// TestSessionBoundRefusesUnfinishableProof: once the estimate exceeds the
// remaining budget, the worker's gate predicate refuses to start another proof
// — the check that stops a session overrunning and starving block import.
func TestSessionBoundRefusesUnfinishableProof(t *testing.T) {
	e := newUnitCostEstimator()
	for range 5 {
		e.observeGroup(2 * time.Second)
	}
	// 500ms left, but a group now costs ~2s: must refuse.
	if 500*time.Millisecond >= e.nextGroupDuration() {
		t.Fatal("expected 500ms remaining to be below the per-group estimate")
	}
	// 3s left: may proceed.
	if 3*time.Second < e.nextGroupDuration() {
		t.Fatal("expected 3s remaining to be above the per-group estimate")
	}
}
