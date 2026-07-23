package blockprocessor

import (
	"errors"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestOnBlockRejectsNilStore(t *testing.T) {
	if err := OnBlock(nil, nil); err == nil {
		t.Fatal("expected nil store error")
	}
}

func TestOnBlockRejectsNilStoreBackend(t *testing.T) {
	err := OnBlockWithoutVerification(&store.ConsensusStore{}, &types.SignedBlock{
		Block: &types.Block{Body: &types.BlockBody{}},
	})
	if err == nil {
		t.Fatal("expected nil backend error")
	}
	if !strings.Contains(err.Error(), "backend") {
		t.Fatalf("error=%v, want backend context", err)
	}
}

func TestOnBlockWithoutVerificationPersistsBlock(t *testing.T) {
	s, parentState, parentRoot := processorStoreWithParent(t)
	block := processorEmptyBlockWithStateRoot(t, parentState, parentRoot)

	if err := OnBlockWithoutVerification(s, &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{},
	}); err != nil {
		t.Fatalf("process block: %v", err)
	}

	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash block: %v", err)
	}
	if !s.HasState(blockRoot) {
		t.Fatal("post-state was not persisted")
	}
	header := s.GetBlockHeader(blockRoot)
	if header == nil {
		t.Fatal("block header was not persisted")
	}
	if header.StateRoot != block.StateRoot {
		t.Fatalf("header state root=0x%x, want 0x%x", header.StateRoot, block.StateRoot)
	}
	if signed := s.GetSignedBlock(blockRoot); signed == nil || signed.Block == nil {
		t.Fatal("signed block was not persisted")
	}
}

func TestOnBlockWithoutVerificationReturnsPersistenceError(t *testing.T) {
	s, parentState, parentRoot := processorStoreWithParent(t)
	block := processorEmptyBlockWithStateRoot(t, parentState, parentRoot)
	s.Backend = failingProcessorWriteBackend{InMemoryBackend: s.Backend.(*storage.InMemoryBackend)}

	err := OnBlockWithoutVerification(s, &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{},
	})
	if err == nil {
		t.Fatal("expected persistence error")
	}
	if !strings.Contains(err.Error(), "begin write") {
		t.Fatalf("error=%v, want begin write context", err)
	}
}

func TestPersistBlockUsesSingleBatch(t *testing.T) {
	s, parentState, parentRoot := processorStoreWithParent(t)
	block := processorEmptyBlockWithStateRoot(t, parentState, parentRoot)
	postState := processorPostState(t, parentState, block)
	blockRoot := processorBlockRoot(t, block)
	s.Backend = putFailingProcessorBackend{
		InMemoryBackend: s.Backend.(*storage.InMemoryBackend),
		failAfter:       1,
	}

	err := persistBlock(s, blockRoot, &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{},
	}, postState)
	if err == nil {
		t.Fatal("expected batch write error")
	}
	if s.GetBlockHeader(blockRoot) != nil {
		t.Fatal("block header was partially persisted")
	}
	if s.HasState(blockRoot) {
		t.Fatal("post-state was partially persisted")
	}
	if s.GetSignedBlock(blockRoot) != nil {
		t.Fatal("signed block was partially persisted")
	}
}

func TestPersistBlockWritesCheckpointMetadata(t *testing.T) {
	s, parentState, parentRoot := processorStoreWithParent(t)
	block := processorEmptyBlockWithStateRoot(t, parentState, parentRoot)
	postState := processorPostState(t, parentState, block)
	blockRoot := processorBlockRoot(t, block)
	postState.LatestJustified = &types.Checkpoint{Slot: 1, Root: blockRoot}
	postState.LatestFinalized = &types.Checkpoint{Slot: 1, Root: blockRoot}

	finalizedBefore := s.LatestFinalized()

	err := persistBlock(s, blockRoot, &types.SignedBlock{
		Block: block,
		Proof: &types.MultiMessageAggregate{},
	}, postState)
	if err != nil {
		t.Fatalf("persist block: %v", err)
	}
	if got := s.LatestJustified(); got == nil || got.Slot != 1 || got.Root != blockRoot {
		t.Fatalf("latest justified=%v, want slot 1 root 0x%x", got, blockRoot)
	}
	// Finalization is derived from the canonical head during head selection,
	// never persisted by the import path, so this write must leave it untouched.
	if got := s.LatestFinalized(); !checkpointEqual(got, finalizedBefore) {
		t.Fatalf("latest finalized=%v, want unchanged %v", got, finalizedBefore)
	}
}

func checkpointEqual(a, b *types.Checkpoint) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Slot == b.Slot && a.Root == b.Root
}

type failingProcessorWriteBackend struct {
	*storage.InMemoryBackend
}

func (b failingProcessorWriteBackend) BeginWrite() (storage.WriteBatch, error) {
	return nil, errors.New("write failed")
}

type putFailingProcessorBackend struct {
	*storage.InMemoryBackend
	failAfter int
}

func (b putFailingProcessorBackend) BeginWrite() (storage.WriteBatch, error) {
	wb, err := b.InMemoryBackend.BeginWrite()
	if err != nil {
		return nil, err
	}
	return &putFailingProcessorBatch{WriteBatch: wb, failAfter: b.failAfter}, nil
}

type putFailingProcessorBatch struct {
	storage.WriteBatch
	failAfter int
	calls     int
}

func (b *putFailingProcessorBatch) PutBatch(table storage.Table, entries []storage.KV) error {
	b.calls++
	if b.calls > b.failAfter {
		return errors.New("put failed")
	}
	return b.WriteBatch.PutBatch(table, entries)
}

func processorStoreWithParent(t *testing.T) (*store.ConsensusStore, *types.State, [32]byte) {
	t.Helper()

	parentState := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     0,
		LatestBlockHeader:        &types.BlockHeader{Slot: 0},
		LatestJustified:          &types.Checkpoint{Slot: 0},
		LatestFinalized:          &types.Checkpoint{Slot: 0},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
		Validators:               []*types.Validator{{Index: 0}},
	}

	stateRoot, err := parentState.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash parent state: %v", err)
	}
	parentState.LatestBlockHeader.StateRoot = stateRoot

	parentRoot, err := parentState.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash parent header: %v", err)
	}

	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	s.InsertState(parentRoot, parentState)
	s.InsertBlockHeader(parentRoot, parentState.LatestBlockHeader)
	s.SetHead(parentRoot)
	s.SetLatestJustified(&types.Checkpoint{Root: parentRoot, Slot: 0})
	s.SetLatestFinalized(&types.Checkpoint{Root: parentRoot, Slot: 0})
	return s, parentState, parentRoot
}

func processorEmptyBlockWithStateRoot(t *testing.T, parentState *types.State, parentRoot [32]byte) *types.Block {
	t.Helper()

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 0,
		ParentRoot:    parentRoot,
		Body:          &types.BlockBody{},
	}

	trial, err := parentState.Clone()
	if err != nil {
		t.Fatalf("clone state: %v", err)
	}
	if err := statetransition.ProcessSlots(trial, block.Slot); err != nil {
		t.Fatalf("process slots: %v", err)
	}
	if err := statetransition.ProcessBlock(trial, block); err != nil {
		t.Fatalf("process block: %v", err)
	}
	stateRoot, err := trial.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash post state: %v", err)
	}
	block.StateRoot = stateRoot
	return block
}

func processorPostState(t *testing.T, parentState *types.State, block *types.Block) *types.State {
	t.Helper()

	postState, err := transitionState(parentState, block)
	if err != nil {
		t.Fatalf("transition state: %v", err)
	}
	postState.LatestBlockHeader.StateRoot = block.StateRoot
	return postState
}

func processorBlockRoot(t *testing.T, block *types.Block) [32]byte {
	t.Helper()

	root, err := block.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash block: %v", err)
	}
	return root
}
