package xmss

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestProposerSigThroughBlockSSZ(t *testing.T) {
	kp, err := GenerateKeyPair("block-roundtrip-0", 0, 1<<18)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	defer kp.Close()

	pkBytes, err := kp.PublicKeyBytes()
	if err != nil {
		t.Fatalf("pubkey: %v", err)
	}

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 0,
		Body:          &types.BlockBody{},
	}
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		t.Fatalf("block root: %v", err)
	}

	sig, err := kp.Sign(1, blockRoot)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	csig, err := ParseSignature(sig[:])
	if err != nil {
		t.Fatalf("parse sig: %v", err)
	}
	defer FreeSignature(csig)

	cpk, err := ParsePublicKey(pkBytes)
	if err != nil {
		t.Fatalf("parse pk: %v", err)
	}
	defer FreePublicKey(cpk)

	proof, err := AggregateSignatures([]CPubKey{cpk}, []CSig{csig}, blockRoot, 1)
	if err != nil {
		t.Fatalf("aggregate FAILED: %v", err)
	}
	merged, err := MergeType1Proofs([]Type1Input{{Pubkeys: []CPubKey{cpk}, Proof: proof}})
	if err != nil {
		t.Fatalf("merge FAILED: %v", err)
	}
	signedBlock := &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{Proof: merged},
	}
	encoded, err := signedBlock.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	decoded := &types.SignedBlock{}
	if err := decoded.UnmarshalSSZ(encoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if err := VerifyType2Proof(
		decoded.Proof.Proof,
		[][]CPubKey{{cpk}},
		[]MessageBinding{{Message: blockRoot, Slot: 1}},
	); err != nil {
		t.Fatalf("verify FAILED: %v", err)
	}
}
