package aggregation

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

// The signatures are only decoded: the injected prover tests scheduling and
// result retention, not cryptographic validity.
func budgetTestSnapshot() *Snapshot {
	snap := aggregateTestSnapshot(5, 6, 7)
	snap.headState.Validators = []*types.Validator{{Index: 0}, {Index: 1}, {Index: 2}}
	for _, dr := range [][32]byte{rootByte(2), rootByte(3)} {
		snap.attSigs[dr].Signatures = []store.AttestationSignatureEntry{{ValidatorID: 0}, {ValidatorID: 1}, {ValidatorID: 2}}
	}
	return snap
}

func TestAggregationBudgetRecovery(t *testing.T) {
	for _, tc := range []struct {
		name         string
		observations []time.Duration
		fail         bool
	}{
		{"first_slow_group", []time.Duration{2 * time.Second}, false},
		{"steady_then_spike", []time.Duration{1400 * time.Millisecond, 2500 * time.Millisecond}, false},
		{"failed_attempt", []time.Duration{2 * time.Second}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				e := newUnitCostEstimator()
				for _, d := range tc.observations {
					e.observeGroup(d)
				}
				e.perUnitSeconds = 1
				cache := xmss.NewPubKeyCache()
				defer cache.Close()
				for session := 0; session < 5; session++ {
					before := e.nextGroupDuration()
					calls := 0
					prove := func(pks []xmss.CPubKey, sigs []xmss.CSig, children []xmss.ChildProof, root [32]byte, slot uint32) ([]byte, error) {
						calls++
						if slot != 6 || len(pks) != 2 || len(sigs) != 2 || len(children) != 0 {
							t.Fatalf("unexpected inputs: slot=%d raw=%d children=%d", slot, len(sigs), len(children))
						}
						time.Sleep(time.Second)
						if tc.fail {
							return nil, errors.New("test prover failed")
						}
						return []byte{1}, nil
					}
					aggs, payloads, deletes, truncated, skips := aggregateFromSnapshotWithProver(budgetTestSnapshot(), cache, time.Now().Add(SessionBudget), MaxGroupsPerSession, shadow.Rates{}, e, prove)
					if calls != 1 || !truncated || skips[metrics.AggGroupSkipBudget] != 1 || skips[metrics.AggGroupSkipTooFewSigners] != 1 {
						t.Fatalf("session=%d calls=%d truncated=%v skips=%v", session, calls, truncated, skips)
					}
					if tc.fail {
						if len(aggs) != 0 || len(payloads) != 0 || len(deletes) != 0 || skips[metrics.AggGroupSkipError] != 1 || e.nextGroupDuration() != before {
							t.Fatalf("failure changed results/estimate or lost skip: %v", skips)
						}
					} else {
						if len(aggs) != 1 || len(payloads) != 1 || len(deletes) != 2 {
							t.Fatalf("lost partial results: aggs=%d payloads=%d deletes=%d", len(aggs), len(payloads), len(deletes))
						}
						if e.nextGroupDuration() >= before {
							t.Fatal("successful attempt did not recalibrate")
						}
						if types.BitlistCount(aggs[0].Proof.Participants) != 2 || deletes[0].ValidatorID != 0 || deletes[1].ValidatorID != 1 {
							t.Fatal("deferred signature included or retired")
						}
					}
				}
			})
		})
	}
}

func TestAggregationBudgetDeadline(t *testing.T) {
	for _, tc := range []struct {
		name          string
		deadline      time.Duration
		proofTime     time.Duration
		wantCalls     int
		wantTruncated bool
	}{
		{"expired", -time.Second, 0, 0, true},
		{"at_deadline", 0, 0, 0, true},
		{"first_proof_overruns", SessionBudget, 2 * time.Second, 1, true},
		{"enough_for_later_group", SessionBudget, 100 * time.Millisecond, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cache := xmss.NewPubKeyCache()
				defer cache.Close()
				calls := 0
				prove := func([]xmss.CPubKey, []xmss.CSig, []xmss.ChildProof, [32]byte, uint32) ([]byte, error) {
					calls++
					time.Sleep(tc.proofTime)
					return []byte{1}, nil
				}
				aggs, _, _, truncated, skips := aggregateFromSnapshotWithProver(budgetTestSnapshot(), cache, time.Now().Add(tc.deadline), MaxGroupsPerSession, shadow.Rates{}, newUnitCostEstimator(), prove)
				if calls != tc.wantCalls || len(aggs) != calls || truncated != tc.wantTruncated {
					t.Fatalf("calls=%d aggs=%d truncated=%v skips=%v", calls, len(aggs), truncated, skips)
				}
			})
		})
	}
}
