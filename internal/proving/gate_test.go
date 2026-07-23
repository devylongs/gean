package proving

import (
	"context"
	"testing"
	"time"
)

func TestGateRejectsBackgroundWhileProposalPending(t *testing.T) {
	gate := NewGate()
	if !gate.Acquire(context.Background(), false) {
		t.Fatal("first background acquire failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if gate.Acquire(ctx, true) {
		t.Fatal("canceled proposal acquired gate")
	}
	gate.proposalPending.Store(true)
	if gate.Acquire(context.Background(), false) {
		t.Fatal("background acquired while proposal pending")
	}
	gate.proposalPending.Store(false)
	gate.Release(false)
}

// The gate must stay live and conserve its token under concurrent proposal
// cycles and background waiters. Single-goroutine tests cannot exercise the
// interleaving between Release and a parked Acquire (see Release's ordering
// comment), so this stresses the real concurrent paths under the race detector.
func TestGateConcurrentProposalAndBackground(t *testing.T) {
	gate := NewGate()
	const cycles = 2000
	done := make(chan struct{})

	go func() {
		defer close(done)
		for range cycles {
			if gate.Acquire(context.Background(), true) {
				gate.Release(true)
			}
		}
	}()

	backgroundAcquired := 0
	timeout := time.After(30 * time.Second)
	for range cycles {
		select {
		case <-timeout:
			t.Fatal("gate stress timed out: deadlock or lost token")
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		if gate.Acquire(ctx, false) {
			backgroundAcquired++
			gate.Release(false)
		}
		cancel()
	}
	<-done

	// Token conserved and priority flag cleared after all cycles.
	if !gate.Acquire(context.Background(), false) {
		t.Fatal("background acquire failed after stress: stale proposalPending or lost token")
	}
	gate.Release(false)
	if !gate.Acquire(context.Background(), true) {
		t.Fatal("proposal acquire failed after stress: lost token")
	}
	gate.Release(true)
	if backgroundAcquired == 0 {
		t.Fatal("background waiter never acquired the gate during stress")
	}
}
