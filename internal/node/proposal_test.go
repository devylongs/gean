package node

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func proposalHeadState(t *testing.T) (*types.State, [32]byte) {
	t.Helper()

	state := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     0,
		LatestBlockHeader:        &types.BlockHeader{Slot: 0},
		LatestJustified:          &types.Checkpoint{Slot: 0},
		LatestFinalized:          &types.Checkpoint{Slot: 0},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
		Validators:               []*types.Validator{{}},
	}

	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute state root: %v", err)
	}
	state.LatestBlockHeader.StateRoot = stateRoot

	parentRoot, err := state.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute parent root: %v", err)
	}
	return state, parentRoot
}

func TestProduceBlockWithSignaturesDoesNotPromoteNewPayloads(t *testing.T) {
	s := makeTestStore()
	headState, parentRoot := proposalHeadState(t)
	s.SetHead(parentRoot)
	s.InsertState(parentRoot, headState)
	s.InsertBlockHeader(parentRoot, headState.LatestBlockHeader)

	data := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Slot: 0, Root: parentRoot},
		Source: &types.Checkpoint{Slot: 0, Root: parentRoot},
		Target: &types.Checkpoint{Slot: 0, Root: parentRoot},
	}
	dataRoot, err := data.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash attestation data: %v", err)
	}
	s.NewPayloads.Push(dataRoot, data, &types.SingleMessageAggregate{
		Participants: types.BitlistFromIndices([]uint64{0}),
		Proof:        []byte{0x01},
	})

	e := &Engine{Store: s}
	block, sigs, err := e.produceBlockWithSignatures(1, 0)
	if err != nil {
		t.Fatalf("produce block: %v", err)
	}
	if block == nil {
		t.Fatal("expected produced block")
	}
	if len(sigs) != 0 {
		t.Fatalf("signature proofs=%d, want 0", len(sigs))
	}
	if s.NewPayloads.Len() != 1 || s.KnownPayloads.Len() != 0 {
		t.Fatalf("payload promotion changed buffers: new=%d known=%d", s.NewPayloads.Len(), s.KnownPayloads.Len())
	}
}

func TestPayloadsFromEntriesCopiesMapAndProofSlice(t *testing.T) {
	root := [32]byte{0x01}
	entry := &store.PayloadEntry{
		Data:   &types.AttestationData{Head: &types.Checkpoint{}, Source: &types.Checkpoint{}, Target: &types.Checkpoint{}},
		Proofs: []*types.SingleMessageAggregate{{Participants: types.BitlistFromIndices([]uint64{1})}},
	}
	original := map[[32]byte]*store.PayloadEntry{
		root: entry,
	}

	copied := payloadsFromEntries(original)
	if len(copied) != 1 {
		t.Fatalf("payloads=%d, want 1", len(copied))
	}
	copied[0].Proofs[0] = nil
	if entry.Proofs[0] == nil {
		t.Fatal("copied proof slice aliases original slice")
	}

	delete(original, root)

	if copied[0].DataRoot != root || copied[0].Data == nil {
		t.Fatal("copied payload lost entry after original map mutation")
	}
}

func TestProduceBlockWithSignaturesStaleHeadReturnsSlotError(t *testing.T) {
	s := makeTestStore()
	headState, parentRoot := proposalHeadState(t)
	headState.Slot = 1
	s.SetHead(parentRoot)
	s.InsertState(parentRoot, headState)
	s.InsertBlockHeader(parentRoot, headState.LatestBlockHeader)

	e := &Engine{Store: s}
	_, _, err := e.produceBlockWithSignatures(1, 0)
	var stale *statetransition.StateSlotIsNewerError
	if !errors.As(err, &stale) {
		t.Fatalf("err=%v, want StateSlotIsNewerError", err)
	}
}

func TestBuildProposalSkipsStaleSlot(t *testing.T) {
	s := makeTestStore()
	headState, parentRoot := proposalHeadState(t)
	headState.Slot = 1
	s.SetHead(parentRoot)
	s.InsertState(parentRoot, headState)
	s.InsertBlockHeader(parentRoot, headState.LatestBlockHeader)

	e := &Engine{Store: s}
	if result := e.buildProposal(1, 0); result != nil {
		t.Fatalf("buildProposal result=%v, want nil (skipped)", result)
	}
}

func TestProduceBlockWithSignaturesRejectsNonProposer(t *testing.T) {
	s := makeTestStore()
	headState, parentRoot := proposalHeadState(t)
	headState.Validators = []*types.Validator{{}, {}}
	s.SetHead(parentRoot)
	s.InsertState(parentRoot, headState)
	s.InsertBlockHeader(parentRoot, headState.LatestBlockHeader)

	e := &Engine{Store: s}
	block, sigs, err := e.produceBlockWithSignatures(1, 0)
	if err == nil {
		t.Fatal("expected non-proposer error")
	}
	if block != nil || sigs != nil {
		t.Fatalf("expected nil block and signatures, got block=%v sigs=%v", block, sigs)
	}
}
