package blockprocessor

import (
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestVerifyBlockSignaturesRejectsMissingProof(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	err := verifyBlockSignatures(s, &types.SignedBlock{
		Block: processorBlock(),
		Proof: &types.MultiMessageAggregate{},
	}, &types.State{Validators: []*types.Validator{{}}})
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrAttestationSignatureMismatch {
		t.Fatalf("expected missing proof error, got %v", err)
	}
}

func TestVerifyBlockSignaturesRejectsEmptyParticipants(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	err := verifyBlockSignatures(s, &types.SignedBlock{
		Block: processorBlock(processorAttestation()),
		Proof: &types.MultiMessageAggregate{Proof: []byte{1}},
	}, &types.State{Validators: []*types.Validator{{}}})
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrParticipantsMismatch {
		t.Fatalf("expected participant mismatch error, got %v", err)
	}
}

func TestVerifyBlockSignaturesRejectsAttestationSlotOverflow(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	att := processorAttestation()
	att.Data.Slot = ^uint64(0)
	types.BitlistSet(att.AggregationBits, 0)
	err := verifyBlockSignatures(s, &types.SignedBlock{
		Block: processorBlock(att),
		Proof: &types.MultiMessageAggregate{Proof: []byte{1}},
	}, &types.State{Validators: []*types.Validator{{}}})
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrSignatureDecodingFailed {
		t.Fatalf("expected slot decoding error, got %v", err)
	}
}

func TestVerifyBlockSignaturesRejectsBlockSlotOverflow(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	block := processorBlock()
	block.Slot = ^uint64(0)
	err := verifyBlockSignatures(s, &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{Proof: []byte{1}},
	}, &types.State{Validators: []*types.Validator{{}}})
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrProposerSignatureDecodingFailed {
		t.Fatalf("expected proposer slot decoding error, got %v", err)
	}
}

func TestVerifyBlockSignaturesRejectsNilProposerValidator(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	err := verifyBlockSignatures(s, &types.SignedBlock{
		Block: processorBlock(),
		Proof: &types.MultiMessageAggregate{Proof: []byte{1}},
	}, &types.State{Validators: []*types.Validator{nil}})
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrInvalidValidatorIndex {
		t.Fatalf("expected invalid validator index error, got %v", err)
	}
}
