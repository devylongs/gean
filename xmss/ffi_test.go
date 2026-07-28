package xmss

import (
	"errors"
	"testing"
	"unsafe"
)

func TestParsePublicKeyRoundtrip(t *testing.T) {
	kp, err := GenerateKeyPair("gean-parse-pk-seed", 0, 1<<10)
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}
	defer kp.Close()

	pubkey, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey serialization failed: %v", err)
	}

	pk, err := ParsePublicKey(pubkey)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer FreePublicKey(pk)

	if pk == nil {
		t.Fatal("expected non-nil public key")
	}
}

func TestParseSignatureRoundtrip(t *testing.T) {
	kp, err := GenerateKeyPair("gean-parse-sig-seed", 0, 1<<10)
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}
	defer kp.Close()

	var message [32]byte
	message[0] = 0x11

	sig, err := kp.Sign(0, message)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	parsed, err := ParseSignature(sig[:])
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	defer FreeSignature(parsed)

	if parsed == nil {
		t.Fatal("expected non-nil signature")
	}
}

func TestKeyGenerateSignVerifyRoundtrip(t *testing.T) {
	kp, err := GenerateKeyPair("gean-test-seed-phrase", 0, 1<<10)
	if err != nil {
		t.Fatalf("key generation failed: %v", err)
	}
	defer kp.Close()

	pubkey, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey serialization failed: %v", err)
	}

	var message [32]byte
	message[0] = 0xab
	message[31] = 0xcd

	sig, err := kp.Sign(0, message)
	if err != nil {
		t.Fatalf("sign failed: %v", err)
	}

	valid, err := VerifySignatureSSZ(pubkey, 0, message, sig)
	if err != nil {
		t.Fatalf("verify error: %v", err)
	}
	if !valid {
		t.Fatal("signature should be valid")
	}

	valid, err = VerifySignatureSSZ(pubkey, 1, message, sig)
	if err != nil {
		t.Fatalf("verify error: %v", err)
	}
	if valid {
		t.Fatal("signature should be invalid with wrong slot")
	}

	var wrongMsg [32]byte
	wrongMsg[0] = 0xff
	valid, err = VerifySignatureSSZ(pubkey, 0, wrongMsg, sig)
	if err != nil {
		t.Fatalf("verify error: %v", err)
	}
	if valid {
		t.Fatal("signature should be invalid with wrong message")
	}
}

func TestVerifySignatureSSZMalformedPubkey(t *testing.T) {
	var pubkey [32]byte
	var sig [1208]byte
	var message [32]byte

	_, err := VerifySignatureSSZ(pubkey, 0, message, sig)
	if err != nil {
		return
	}
}

func TestAggregateWithChildrenRejectsMalformedChildProof(t *testing.T) {
	var message [32]byte
	marker := byte(1)
	pubkey := CPubKey(unsafe.Pointer(&marker))

	_, err := AggregateWithChildren(nil, nil, []ChildProof{
		{Pubkeys: []CPubKey{pubkey}},
		{Pubkeys: []CPubKey{pubkey}, Proof: []byte{0x01}},
	}, message, 0)
	if !errors.Is(err, ErrMalformedChildProof) {
		t.Fatalf("error=%v, want ErrMalformedChildProof", err)
	}
}

func TestAggregateRejectsNilRawInputs(t *testing.T) {
	var message [32]byte
	marker := byte(1)
	pubkey := CPubKey(unsafe.Pointer(&marker))

	if _, err := AggregateSignatures([]CPubKey{nil}, []CSig{CSig(unsafe.Pointer(&marker))}, message, 0); !errors.Is(err, ErrMalformedRawInput) {
		t.Fatalf("nil pubkey error=%v, want ErrMalformedRawInput", err)
	}
	if _, err := AggregateSignatures([]CPubKey{pubkey}, []CSig{nil}, message, 0); !errors.Is(err, ErrMalformedRawInput) {
		t.Fatalf("nil signature error=%v, want ErrMalformedRawInput", err)
	}
}
