package node

import (
	"context"

	"github.com/geanlabs/gean/internal/aggregation"
)

func (e *Engine) startWorkers(ctx context.Context) {
	go e.runFetchBatcher(ctx)
	go aggregation.RunWorker(ctx, e.AggregationDispatchCh, e.Store, e.Store.PubKeyCache, e.P2P, e.ProvingGate, e.Shadow)
	go e.runProposalWorker(ctx)
	go e.runRecoveryWorker(ctx)
	go e.runAttestationWorker(ctx)
	go e.runAggregationWorker(ctx)
	go e.runGossipMeshGauge(ctx)
}

func (e *Engine) runAttestationWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case att := <-e.AttestationCh:
			go e.onGossipAttestation(att)
		}
	}
}

// runAggregationWorker verifies gossiped aggregates off the dispatch loop.
// Verification is a recursive XMSS proof check — the most expensive verify gean
// does — and on the loop it delayed store.OnTick, stalling the store clock.
// Unlike single attestations these are not fanned out per message: the check is
// CPU-bound, so serialising it here bounds the cost instead of thrashing. The
// buffers it writes are mutex-protected, so a worker goroutine is safe.
func (e *Engine) runAggregationWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case agg := <-e.AggregationCh:
			e.onGossipAggregatedAttestation(agg)
		}
	}
}
