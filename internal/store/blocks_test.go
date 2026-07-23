package store_test

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

type failingReadBackend struct{}

func (failingReadBackend) BeginRead() (storage.ReadView, error) {
	return nil, errors.New("read failed")
}

func (failingReadBackend) BeginWrite() (storage.WriteBatch, error) {
	return nil, errors.New("write failed")
}

func (failingReadBackend) EstimateTableBytes(storage.Table) uint64 {
	return 0
}

func (failingReadBackend) Close() error {
	return nil
}

func TestBlockHeaderStorage(t *testing.T) {
	s := makeTestStore()
	var root [32]byte
	root[0] = 0x01
	h := makeHeader(5, 2, 0x00)

	s.InsertBlockHeader(root, h)
	got := s.GetBlockHeader(root)
	if got == nil {
		t.Fatal("header not found")
	}
	if got.Slot != 5 || got.ProposerIndex != 2 {
		t.Fatalf("header mismatch: slot=%d proposer=%d", got.Slot, got.ProposerIndex)
	}
}

func TestBlockRoots(t *testing.T) {
	s := makeTestStore()
	rootA := [32]byte{0x01}
	rootB := [32]byte{0x02}
	s.InsertBlockHeader(rootA, makeHeader(1, 0, 0x00))
	s.InsertBlockHeader(rootB, makeHeader(2, 0, 0x01))

	roots, err := s.BlockRoots()
	if err != nil {
		t.Fatalf("block roots: %v", err)
	}
	if !roots[rootA] || !roots[rootB] {
		t.Fatalf("missing block roots: %v", roots)
	}
}

func TestBlockRootsReturnsReadError(t *testing.T) {
	s := store.NewConsensusStore(failingReadBackend{})

	roots, err := s.BlockRoots()
	if err == nil {
		t.Fatal("expected read error")
	}
	if roots != nil {
		t.Fatalf("roots=%v, want nil", roots)
	}
}

func TestBlockWritesIgnoreMalformedInput(t *testing.T) {
	s := makeTestStore()
	var root [32]byte
	root[0] = 1

	s.InsertBlockHeader(root, nil)
	if got := s.GetBlockHeader(root); got != nil {
		t.Fatal("nil header should not be stored")
	}

	s.StorePendingBlock(root, nil)
	s.StorePendingBlock(root, &types.SignedBlock{})
	store.WriteBlockData(s, root, nil)

	if got := s.GetSignedBlock(root); got != nil {
		t.Fatal("malformed signed block should not be stored")
	}
}

func TestPutBlockHeaderReturnsInputAndWriteErrors(t *testing.T) {
	s := makeTestStore()
	var root [32]byte
	if err := s.PutBlockHeader(root, nil); err == nil {
		t.Fatal("expected nil header error")
	}

	s = store.NewConsensusStore(failingWriteBackend{InMemoryBackend: storage.NewInMemoryBackend()})
	if err := s.PutBlockHeader(root, makeHeader(1, 0, 0)); err == nil {
		t.Fatal("expected write error")
	}
}

func TestWriteBlockDataReturnsErrors(t *testing.T) {
	var root [32]byte
	if err := store.WriteBlockData(nil, root, makeSignedBlock()); err == nil {
		t.Fatal("expected nil store error")
	}

	s := makeTestStore()
	if err := store.WriteBlockData(s, root, nil); err == nil {
		t.Fatal("expected nil signed block error")
	}
	if err := store.WriteBlockData(s, root, &types.SignedBlock{}); err == nil {
		t.Fatal("expected missing block error")
	}
	if err := store.WriteBlockData(s, root, &types.SignedBlock{Block: &types.Block{}, Proof: &types.MultiMessageAggregate{}}); err == nil {
		t.Fatal("expected missing block body error")
	}
	if got := s.GetSignedBlock(root); got != nil {
		t.Fatal("malformed signed block should not be stored")
	}

	s = store.NewConsensusStore(failingWriteBackend{InMemoryBackend: storage.NewInMemoryBackend()})
	if err := store.WriteBlockData(s, root, makeSignedBlock()); err == nil {
		t.Fatal("expected write error")
	}
}

func TestStorePendingBlockReturnsErrors(t *testing.T) {
	var root [32]byte
	s := makeTestStore()
	if err := s.StorePendingBlock(root, nil); err == nil {
		t.Fatal("expected nil signed block error")
	}
	if err := s.StorePendingBlock(root, &types.SignedBlock{}); err == nil {
		t.Fatal("expected missing block error")
	}

	s = store.NewConsensusStore(failingWriteBackend{InMemoryBackend: storage.NewInMemoryBackend()})
	if err := s.StorePendingBlock(root, makeSignedBlock()); err == nil {
		t.Fatal("expected write error")
	}
}

func makeSignedBlock() *types.SignedBlock {
	return &types.SignedBlock{
		Block: &types.Block{
			Slot: 1,
			Body: &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{},
	}
}
