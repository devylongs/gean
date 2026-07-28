package xmss

import (
	"testing"
)

func TestMultipleAggregationsSequential(t *testing.T) {
	kp1, err := GenerateKeyPair("multi-agg-0", 0, 1<<10)
	if err != nil {
		t.Fatalf("keygen 0: %v", err)
	}
	defer kp1.Close()
	kp2, err := GenerateKeyPair("multi-agg-1", 0, 1<<10)
	if err != nil {
		t.Fatalf("keygen 1: %v", err)
	}
	defer kp2.Close()

	pk1, err := kp1.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey 0: %v", err)
	}
	pk2, err := kp2.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey 1: %v", err)
	}

	EnsureProverReady()
	EnsureVerifierReady()

	for slot := range uint32(5) {
		var message [32]byte
		message[0] = byte(slot)
		message[1] = 0xab

		sig1, err := kp1.Sign(slot, message)
		if err != nil {
			t.Fatalf("slot %d sign kp1: %v", slot, err)
		}
		sig2, err := kp2.Sign(slot, message)
		if err != nil {
			t.Fatalf("slot %d sign kp2: %v", slot, err)
		}

		csig1, err := ParseSignature(sig1[:])
		if err != nil {
			t.Fatalf("slot %d parse sig1: %v", slot, err)
		}
		defer FreeSignature(csig1)
		csig2, err := ParseSignature(sig2[:])
		if err != nil {
			t.Fatalf("slot %d parse sig2: %v", slot, err)
		}
		defer FreeSignature(csig2)

		cpk1, err := ParsePublicKey(pk1)
		if err != nil {
			t.Fatalf("slot %d parse pk1: %v", slot, err)
		}
		defer FreePublicKey(cpk1)
		cpk2, err := ParsePublicKey(pk2)
		if err != nil {
			t.Fatalf("slot %d parse pk2: %v", slot, err)
		}
		defer FreePublicKey(cpk2)

		proof, err := AggregateSignatures(
			[]CPubKey{cpk1, cpk2},
			[]CSig{csig1, csig2},
			message,
			slot,
		)
		if err != nil {
			t.Fatalf("slot %d aggregate FAILED: %v", slot, err)
		}

		err = VerifyAggregatedSignature(proof, []CPubKey{cpk1, cpk2}, message, slot)
		if err != nil {
			t.Fatalf("slot %d verify FAILED: %v", slot, err)
		}

		t.Logf("slot %d: aggregate + verify OK (proof=%d bytes)", slot, len(proof))
	}
}
