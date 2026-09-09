package node

import (
	"context"
	"testing"
	"time"
)

// Shutdown cancels the context, waits a fixed grace period, and then closes the
// storage backend. Nothing joined the background samplers, so one still mid-round
// when Close landed would call into a closed Pebble instance — which panics
// rather than returning an error.
//
// WaitForStorageWorkers is what makes the ordering safe, so it has to actually
// block until the sampler has returned.
func TestWaitForStorageWorkersBlocksUntilSamplerReturns(t *testing.T) {
	e := makeTestEngine()
	ctx, cancel := context.WithCancel(context.Background())

	e.startWorkers(ctx)

	done := make(chan struct{})
	go func() {
		e.WaitForStorageWorkers()
		close(done)
	}()

	// Nothing has been cancelled, so the sampler is still running and the wait
	// must not have returned.
	select {
	case <-done:
		t.Fatal("WaitForStorageWorkers returned while the sampler was still running")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForStorageWorkers did not return after cancellation")
	}
}
