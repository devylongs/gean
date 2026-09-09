//go:build aggregation_diagnostic

package aggregation

import (
	"fmt"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
	"testing"
	"time"
)

func TestRawFirstCrypto(t *testing.T) {
	snap := aggregateTestSnapshot(6)
	dr := rootByte(1)
	message, slot, err := aggregationMessage(snap.attSigs[dr].Data)
	if err != nil {
		t.Fatal(err)
	}
	var pks []xmss.CPubKey
	var sigs []xmss.CSig
	var entries []store.AttestationSignatureEntry
	for i := 0; i < 3; i++ {
		kp, err := xmss.GenerateKeyPair(fmt.Sprintf("raw-first-test-%d", i), 0, 1<<10)
		if err != nil {
			t.Fatal(err)
		}
		defer kp.Close()
		pkBytes, err := kp.PublicKeyBytes()
		if err != nil {
			t.Fatal(err)
		}
		sigBytes, err := kp.Sign(slot, message)
		if err != nil {
			t.Fatal(err)
		}
		validator := &types.Validator{Index: uint64(i)}
		copy(validator.AttestationPubkey[:], pkBytes[:])
		snap.headState.Validators = append(snap.headState.Validators, validator)
		entries = append(entries, store.AttestationSignatureEntry{ValidatorID: uint64(i), Signature: sigBytes})
		pk, err := xmss.ParsePublicKey(pkBytes)
		if err != nil {
			t.Fatal(err)
		}
		defer xmss.FreePublicKey(pk)
		sig, err := xmss.ParseSignature(sigBytes[:])
		if err != nil {
			t.Fatal(err)
		}
		defer xmss.FreeSignature(sig)
		pks = append(pks, pk)
		sigs = append(sigs, sig)
	}
	child, err := xmss.AggregateSignatures(pks[:2], sigs[:2], message, slot)
	if err != nil {
		t.Fatal(err)
	}
	snap.newEntries[dr] = &store.PayloadEntry{Proofs: []*types.SingleMessageAggregate{{Participants: types.BitlistFromIndices([]uint64{0, 1}), Proof: child}}}
	cache := xmss.NewPubKeyCache()
	defer cache.Close()
	for _, overlap := range []bool{false, true} {
		snap.attSigs[dr].Signatures = entries
		wantRaw, wantChild := 3, 0
		if overlap {
			snap.attSigs[dr].Signatures = []store.AttestationSignatureEntry{entries[0], entries[2]}
			wantRaw, wantChild = 1, 1
		}
		prove := func(rawPKs []xmss.CPubKey, rawSigs []xmss.CSig, children []xmss.ChildProof, msg [32]byte, s uint32) ([]byte, error) {
			if len(rawSigs) != wantRaw || len(children) != wantChild {
				t.Fatalf("raw=%d children=%d", len(rawSigs), len(children))
			}
			return xmss.AggregateWithChildren(rawPKs, rawSigs, children, msg, s)
		}
		aggs, _, deletes, _, skips := aggregateFromSnapshotWithProver(snap, cache, time.Now().Add(30*time.Second), MaxGroupsPerSession, shadow.Rates{}, newUnitCostEstimator(), prove)
		if len(aggs) != 1 {
			t.Fatalf("no aggregate: %v", skips)
		}
		if types.BitlistCount(aggs[0].Proof.Participants) != 3 || len(deletes) != len(snap.attSigs[dr].Signatures) {
			t.Fatal("incorrect coverage or deletion")
		}
		if err := xmss.VerifyAggregatedSignature(aggs[0].Proof.Proof, pks, message, slot); err != nil {
			t.Fatal(err)
		}
		t.Logf("overlap=%v raw=%d children=%d participants=3 verified=true", overlap, wantRaw, wantChild)
	}
}
