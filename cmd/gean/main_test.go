package main

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestMain(m *testing.M) {
	logger.SetQuiet(true)
	os.Exit(m.Run())
}

func newTestStore() *store.ConsensusStore {
	return store.NewConsensusStore(storage.NewInMemoryBackend())
}

func TestRecoverStoreTime_PostGenesis(t *testing.T) {
	s := newTestStore()
	nowMs := uint64(time.Now().UnixMilli())
	genesisSec := (nowMs / 1000) - 10

	if err := recoverStoreTime(s, genesisSec); err != nil {
		t.Fatalf("recover store time: %v", err)
	}

	got := s.Time()
	expectedMin := uint64(10*1000/types.MillisecondsPerInterval) - 1
	expectedMax := uint64(10*1000/types.MillisecondsPerInterval) + 1
	if got < expectedMin || got > expectedMax {
		t.Fatalf("intervals: expected ~%d, got %d", (expectedMin+expectedMax)/2, got)
	}
}

func TestRecoverStoreTime_PreGenesis(t *testing.T) {
	s := newTestStore()
	s.SetTime(999)

	farFuture := uint64(time.Now().Unix()) + 86400
	if err := recoverStoreTime(s, farFuture); err != nil {
		t.Fatalf("recover store time: %v", err)
	}

	if got := s.Time(); got != 0 {
		t.Fatalf("expected time=0 for pre-genesis wall clock, got %d", got)
	}
}

func TestRecoverStoreTime_OverwritesStaleValue(t *testing.T) {
	s := newTestStore()
	s.SetTime(1)

	genesisSec := uint64(time.Now().Unix()) - 60
	if err := recoverStoreTime(s, genesisSec); err != nil {
		t.Fatalf("recover store time: %v", err)
	}

	if got := s.Time(); got < 70 {
		t.Fatalf("recovered time did not overwrite stale value: got %d, want >=70", got)
	}
}

func TestRecoverStoreTimeRejectsOverflow(t *testing.T) {
	err := recoverStoreTime(newTestStore(), ^uint64(0))
	if err == nil {
		t.Fatal("expected overflow error")
	}
}

func TestInitStoreFromStateRejectsMalformedState(t *testing.T) {
	_, err := initStoreFromState(newTestStore(), &types.State{})
	if err == nil {
		t.Fatal("expected malformed state error")
	}
}

func TestInitStoreFromStatePersistsAnchor(t *testing.T) {
	s := newTestStore()
	state := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1},
		Slot:                     0,
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
		Validators:               []*types.Validator{{}},
	}

	root, err := initStoreFromState(s, state)
	if err != nil {
		t.Fatalf("initialize store: %v", err)
	}
	if got := s.Head(); got != root {
		t.Fatalf("head=%x, want %x", got, root)
	}
	if s.GetBlockHeader(root) == nil {
		t.Fatal("expected persisted block header")
	}
	if s.GetState(root) == nil {
		t.Fatal("expected persisted state")
	}
}

func TestBootstrapStoreRejectsPreDevnet5Database(t *testing.T) {
	s := newTestStore()
	root := [32]byte{0x01}
	s.SetHead(root)
	s.InsertBlockHeader(root, &types.BlockHeader{Slot: 1})
	s.InsertState(root, &types.State{})

	err := bootstrapStore(s, nil, "")
	if err == nil {
		t.Fatal("expected incompatible data directory error")
	}
}

func TestInitStoreFromStateReturnsWriteError(t *testing.T) {
	s := store.NewConsensusStore(failingWriteBackend{})
	state := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1},
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}

	_, err := initStoreFromState(s, state)
	if !errors.Is(err, errTestWriteFailed) {
		t.Fatalf("error=%v, want write failure", err)
	}
}

var errTestWriteFailed = errors.New("write failed")

type failingWriteBackend struct{}

func (failingWriteBackend) BeginRead() (storage.ReadView, error) {
	return nil, errTestWriteFailed
}

func (failingWriteBackend) BeginWrite() (storage.WriteBatch, error) {
	return nil, errTestWriteFailed
}

func (failingWriteBackend) EstimateTableBytes(storage.Table) uint64 {
	return 0
}

func (failingWriteBackend) Close() error {
	return nil
}
