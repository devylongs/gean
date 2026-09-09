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

// A session's cost is otherwise whatever the backlog costs. The cap counts
// proof attempts, so groups dropped before reaching the prover do not consume
// it, and the groups it defers are reported separately from a budget stop.
func TestAggregateFromSnapshotCapsGroupsPerSession(t *testing.T) {
	for _, tc := range []struct {
		name      string
		maxGroups int
		wantProve int
	}{
		{name: "default_cap", maxGroups: MaxGroupsPerSession, wantProve: 2},
		{name: "proposing_next_slot", maxGroups: MaxGroupsWhenProposing, wantProve: 1},
		{name: "zero_falls_back_to_default", maxGroups: 0, wantProve: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snap := aggregateTestSnapshot(5, 6, 7, 8)
			snap.headState.Validators = []*types.Validator{{Index: 0}, {Index: 1}}
			for i := range 4 {
				dr := rootByte(byte(i + 1))
				snap.attSigs[dr].Signatures = []store.AttestationSignatureEntry{
					{ValidatorID: 0}, {ValidatorID: 1},
				}
			}

			cache := xmss.NewPubKeyCache()
			defer cache.Close()

			calls := 0
			prove := func([]xmss.CPubKey, []xmss.CSig, []xmss.ChildProof, [32]byte, uint32) ([]byte, error) {
				calls++
				return []byte{1}, nil
			}

			aggs, _, _, truncated, skips := aggregateFromSnapshotWithProver(
				snap, cache, time.Now().Add(time.Hour), tc.maxGroups, shadow.Rates{}, newUnitCostEstimator(), prove)

			if calls != tc.wantProve {
				t.Fatalf("prove calls = %d, want %d", calls, tc.wantProve)
			}
			if len(aggs) != tc.wantProve {
				t.Fatalf("aggregates = %d, want %d", len(aggs), tc.wantProve)
			}
			if !truncated {
				t.Fatal("expected truncation once the cap stopped the session")
			}
			// A generous deadline was given, so nothing here is a budget stop.
			if got := skips[metrics.AggGroupSkipSessionCap]; got != 4-tc.wantProve {
				t.Fatalf("session_cap skips = %d, want %d", got, 4-tc.wantProve)
			}
			if got := skips[metrics.AggGroupSkipBudget]; got != 0 {
				t.Fatalf("budget skips = %d, want 0", got)
			}
		})
	}
}
