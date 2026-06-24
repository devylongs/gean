package node

import (
	"context"

	"github.com/geanlabs/gean/internal/aggregation"
)

func (e *Engine) startWorkers(ctx context.Context) {
	go e.runFetchBatcher(ctx)
	go aggregation.RunWorker(ctx, e.AggregationDispatchCh, e.Store, e.Store.PubKeyCache, e.P2P, e.ShadowCosts)
	go e.runAttestationWorker(ctx)
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
