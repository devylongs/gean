package xmss

import (
	"testing"
)

func TestAggregateSignaturesRoundtrip(t *testing.T) {
	kp1, err := GenerateKeyPair("agg-test-validator-0", 0, 1<<18)
	if err != nil {
		t.Fatalf("keygen 0: %v", err)
	}
	defer kp1.Close()

	kp2, err := GenerateKeyPair("agg-test-validator-1", 0, 1<<18)
	if err != nil {
		t.Fatalf("keygen 1: %v", err)
	}
	defer kp2.Close()

	var message [32]byte
	message[0] = 0xab

	sig1Bytes, err := kp1.Sign(0, message)
	if err != nil {
		t.Fatalf("sign 0: %v", err)
	}
	sig2Bytes, err := kp2.Sign(0, message)
	if err != nil {
		t.Fatalf("sign 1: %v", err)
	}

	pk1Bytes, err := kp1.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey 1: %v", err)
	}
	pk2Bytes, err := kp2.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey 2: %v", err)
	}

	valid, err := VerifySignatureSSZ(pk1Bytes, 0, message, sig1Bytes)
	if err != nil || !valid {
		t.Fatalf("verify sig1: valid=%v err=%v", valid, err)
	}
	valid, err = VerifySignatureSSZ(pk2Bytes, 0, message, sig2Bytes)
	if err != nil || !valid {
		t.Fatalf("verify sig2: valid=%v err=%v", valid, err)
	}

	cpk1, err := ParsePublicKey(pk1Bytes)
	if err != nil {
		t.Fatalf("parse pk1: %v", err)
	}
	defer FreePublicKey(cpk1)

	cpk2, err := ParsePublicKey(pk2Bytes)
	if err != nil {
		t.Fatalf("parse pk2: %v", err)
	}
	defer FreePublicKey(cpk2)

	csig1, err := ParseSignature(sig1Bytes[:])
	if err != nil {
		t.Fatalf("parse sig1: %v", err)
	}
	defer FreeSignature(csig1)

	csig2, err := ParseSignature(sig2Bytes[:])
	if err != nil {
		t.Fatalf("parse sig2: %v", err)
	}
	defer FreeSignature(csig2)

	EnsureProverReady()

	proofBytes, err := AggregateSignatures(
		[]CPubKey{cpk1, cpk2},
		[]CSig{csig1, csig2},
		message,
		0,
	)
	if err != nil {
		t.Fatalf("aggregate failed: %v", err)
	}
	if len(proofBytes) == 0 {
		t.Fatal("empty proof")
	}

	t.Logf("aggregation succeeded: proof size = %d bytes", len(proofBytes))

	EnsureVerifierReady()

	err = VerifyAggregatedSignature(proofBytes, []CPubKey{cpk1, cpk2}, message, 0)
	if err != nil {
		t.Fatalf("verify aggregated failed: %v", err)
	}

	t.Log("aggregated verification succeeded")
}

// TestAggregateRejectsChildProofWithWrongMessage exercises the fallible
// aggregation path: a structurally valid child proof bound to one message must
// be rejected as a Go error when re-aggregated under a different message, rather
// than panicking across the FFI boundary.
func TestAggregateRejectsChildProofWithWrongMessage(t *testing.T) {
	kpChild, err := GenerateKeyPair("agg-wrongmsg-child", 0, 1<<18)
	if err != nil {
		t.Fatalf("keygen child: %v", err)
	}
	defer kpChild.Close()
	kpRaw, err := GenerateKeyPair("agg-wrongmsg-raw", 0, 1<<18)
	if err != nil {
		t.Fatalf("keygen raw: %v", err)
	}
	defer kpRaw.Close()

	childPk, err := kpChild.PublicKeyBytes()
	if err != nil {
		t.Fatalf("child pubkey: %v", err)
	}
	rawPk, err := kpRaw.PublicKeyBytes()
	if err != nil {
		t.Fatalf("raw pubkey: %v", err)
	}

	var msgA, msgB [32]byte
	msgA[0] = 0xaa
	msgB[0] = 0xbb

	EnsureProverReady()

	// A valid single-signer child proof bound to msgA.
	childSig, err := kpChild.Sign(0, msgA)
	if err != nil {
		t.Fatalf("sign child: %v", err)
	}
	cChildSig, err := ParseSignature(childSig[:])
	if err != nil {
		t.Fatalf("parse child sig: %v", err)
	}
	defer FreeSignature(cChildSig)
	cChildPk, err := ParsePublicKey(childPk)
	if err != nil {
		t.Fatalf("parse child pk: %v", err)
	}
	defer FreePublicKey(cChildPk)

	childProof, err := AggregateSignatures([]CPubKey{cChildPk}, []CSig{cChildSig}, msgA, 0)
	if err != nil {
		t.Fatalf("build child proof: %v", err)
	}

	// A raw signature bound to msgB.
	rawSig, err := kpRaw.Sign(0, msgB)
	if err != nil {
		t.Fatalf("sign raw: %v", err)
	}
	cRawSig, err := ParseSignature(rawSig[:])
	if err != nil {
		t.Fatalf("parse raw sig: %v", err)
	}
	defer FreeSignature(cRawSig)
	cRawPk, err := ParsePublicKey(rawPk)
	if err != nil {
		t.Fatalf("parse raw pk: %v", err)
	}
	defer FreePublicKey(cRawPk)

	// Aggregate under msgB while feeding a child bound to msgA: the aggregator
	// must reject it with an error, never panic.
	_, err = AggregateWithChildren(
		[]CPubKey{cRawPk},
		[]CSig{cRawSig},
		[]ChildProof{{Pubkeys: []CPubKey{cChildPk}, Proof: childProof}},
		msgB,
		0,
	)
	if err == nil {
		t.Fatal("expected error for child proof bound to a different message, got nil")
	}
	t.Logf("rejected mismatched child proof cleanly: %v", err)
}
