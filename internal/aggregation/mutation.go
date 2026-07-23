package aggregation

import "github.com/geanlabs/gean/internal/store"

// Locally-built aggregates land in the new pool; the accept-attestations tick
// promotes them to known, matching leanSpec aggregate (latest_new_aggregated_payloads).
func applyAggregationMutations(s *store.ConsensusStore, payloads []store.PayloadKV, deletes []store.AttestationDeleteKey) {
	s.NewPayloads.PushBatch(payloads)
	s.AttestationSignatures.Delete(deletes)
}
