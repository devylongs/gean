package aggregation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/shadowcost"
	"github.com/geanlabs/gean/internal/types"
)

type recordingPublisher struct {
	count int
	err   error
}

func (p *recordingPublisher) PublishAggregatedAttestation(context.Context, *types.SignedAggregatedAttestation) error {
	p.count++
	return p.err
}

func TestRunWorkerReturnsWhenDispatchChannelCloses(t *testing.T) {
	dispatches := make(chan Dispatch)
	close(dispatches)

	done := make(chan struct{})
	go func() {
		RunWorker(context.Background(), dispatches, nil, nil, nil, shadowcost.Costs{})
		close(done)
	}()

	waitForWorker(t, done)
}

func TestRunWorkerSkipsNilSnapshot(t *testing.T) {
	dispatches := make(chan Dispatch, 1)
	dispatches <- Dispatch{Slot: 1}
	close(dispatches)

	done := make(chan struct{})
	go func() {
		RunWorker(context.Background(), dispatches, nil, nil, nil, shadowcost.Costs{})
		close(done)
	}()

	waitForWorker(t, done)
}

func TestPublishAggregatesContinuesAfterPublishError(t *testing.T) {
	publisher := &recordingPublisher{err: errors.New("publish failed")}
	aggs := []*types.SignedAggregatedAttestation{{}, {}}

	publishAggregates(context.Background(), publisher, aggs)

	if publisher.count != len(aggs) {
		t.Fatalf("published=%d, want %d", publisher.count, len(aggs))
	}
}

func waitForWorker(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not exit")
	}
}
