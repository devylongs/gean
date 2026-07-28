package xmss

import (
	"testing"
)

func TestSignatureSSZRoundtripThenAggregate(t *testing.T) {
	kp, err := GenerateKeyPair("roundtrip-test-0", 0, 1<<10)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	defer kp.Close()

	var message [32]byte
	message[0] = 0xab

	sigBytes, err := kp.Sign(0, message)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	pkBytes, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey: %v", err)
	}
	cpk, err := ParsePublicKey(pkBytes)
	if err != nil {
		t.Fatalf("parse pk: %v", err)
	}
	defer FreePublicKey(cpk)

	csig, err := ParseSignature(sigBytes[:])
	if err != nil {
		t.Fatalf("parse sig from SSZ bytes: %v", err)
	}
	defer FreeSignature(csig)

	EnsureProverReady()
	proofBytes, err := AggregateSignatures(
		[]CPubKey{cpk},
		[]CSig{csig},
		message,
		0,
	)
	if err != nil {
		t.Fatalf("aggregate with SSZ-round-tripped sig FAILED: %v", err)
	}
	t.Logf("aggregate with SSZ-round-tripped sig succeeded: proof=%d bytes", len(proofBytes))

	EnsureVerifierReady()
	err = VerifyAggregatedSignature(proofBytes, []CPubKey{cpk}, message, 0)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	t.Log("verification succeeded")
}
