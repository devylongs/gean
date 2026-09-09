package aggregation

import (
	"errors"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func TestRawFirstSelection(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		raw                   []uint64
		children              [][]uint64
		known, fail           bool
		wantRaw, wantChildren int
		wantIDs               []uint64
	}{
		{name: "redundant_child", raw: []uint64{0, 1, 2}, children: [][]uint64{{0, 1}}, wantRaw: 3, wantIDs: []uint64{0, 1, 2}},
		{name: "redundant_known_child", raw: []uint64{0, 1, 2}, children: [][]uint64{{0, 1}}, known: true, wantRaw: 3, wantIDs: []uint64{0, 1, 2}},
		{name: "overlap_trimmed", raw: []uint64{0, 2}, children: [][]uint64{{0, 1}}, wantRaw: 1, wantChildren: 1, wantIDs: []uint64{0, 1, 2}},
		{name: "missing_raw_uses_child", raw: []uint64{2}, children: [][]uint64{{0, 1}}, wantRaw: 1, wantChildren: 1, wantIDs: []uint64{0, 1, 2}},
		{name: "invalid_child_keeps_raw", raw: []uint64{0, 1}, children: [][]uint64{{0, 9}}, wantRaw: 2, wantIDs: []uint64{0, 1}},
		// Raw signatures are never rationed, so all three are taken; the child is
		// admitted for validator 3, which no signature reaches, and validator 2's
		// signature is then trimmed because the child already covers it.
		{name: "child_covers_trims_its_raw", raw: []uint64{0, 1, 2}, children: [][]uint64{{2, 3}}, wantRaw: 2, wantChildren: 1, wantIDs: []uint64{0, 1, 2, 3}},
		// Stored order would take {0,1} first and then still need {0,1,2} for
		// validator 2, paying for two recursive inputs where one covers everything.
		{name: "greedy_picks_widest_child", raw: []uint64{3}, children: [][]uint64{{0, 1}, {0, 1, 2}}, wantRaw: 1, wantChildren: 1, wantIDs: []uint64{0, 1, 2, 3}},
		{name: "children_only", children: [][]uint64{{0, 1}, {2, 3}}, wantChildren: 2, wantIDs: []uint64{0, 1, 2, 3}},
		{name: "failed_proof_retains_inputs", raw: []uint64{0, 2}, children: [][]uint64{{0, 1}}, fail: true, wantRaw: 1, wantChildren: 1, wantIDs: []uint64{0, 1, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snap := aggregateTestSnapshot(6)
			snap.headState.Validators = []*types.Validator{{Index: 0}, {Index: 1}, {Index: 2}, {Index: 3}}
			dr := rootByte(1)
			for _, id := range tc.raw {
				snap.attSigs[dr].Signatures = append(snap.attSigs[dr].Signatures, store.AttestationSignatureEntry{ValidatorID: id})
			}
			entry := &store.PayloadEntry{}
			for _, ids := range tc.children {
				entry.Proofs = append(entry.Proofs, &types.SingleMessageAggregate{Participants: types.BitlistFromIndices(ids), Proof: []byte{1}})
			}
			if tc.known {
				snap.knownEntries[dr] = entry
			} else {
				snap.newEntries[dr] = entry
			}
			cache := xmss.NewPubKeyCache()
			defer cache.Close()
			estimator := newUnitCostEstimator()
			calls := 0
			prove := func(pks []xmss.CPubKey, sigs []xmss.CSig, children []xmss.ChildProof, _ [32]byte, _ uint32) ([]byte, error) {
				calls++
				if len(pks) != tc.wantRaw || len(sigs) != tc.wantRaw || len(children) != tc.wantChildren {
					t.Fatalf("raw=%d children=%d", len(sigs), len(children))
				}
				if tc.fail {
					return nil, errors.New("test failure")
				}
				return []byte{1}, nil
			}
			aggs, payloads, deletes, _, _ := aggregateFromSnapshotWithProver(snap, cache, time.Now().Add(SessionBudget), MaxGroupsPerSession, shadow.Rates{}, estimator, prove)
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			if tc.fail {
				if len(aggs)+len(payloads)+len(deletes) != 0 {
					t.Fatal("failed proof mutated inputs")
				}
				return
			}
			if len(aggs) != 1 || len(payloads) != 1 {
				t.Fatal("missing output")
			}
			if types.BitlistCount(aggs[0].Proof.Participants) != uint64(len(tc.wantIDs)) {
				t.Fatal("wrong coverage count")
			}
			for _, id := range tc.wantIDs {
				if !types.BitlistGet(aggs[0].Proof.Participants, id) {
					t.Fatalf("missing signer %d", id)
				}
			}
			wantDeletes := 0
			for _, id := range tc.raw {
				for _, represented := range tc.wantIDs {
					if id == represented {
						wantDeletes++
					}
				}
			}
			if len(deletes) != wantDeletes {
				t.Fatalf("deletes=%d want=%d", len(deletes), wantDeletes)
			}
			for _, key := range deletes {
				if !types.BitlistGet(aggs[0].Proof.Participants, key.ValidatorID) {
					t.Fatal("deleted deferred signature")
				}
			}
		})
	}
}
