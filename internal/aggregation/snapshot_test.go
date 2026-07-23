package aggregation

import (
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestSnapshotInputsCapturesPayloadAndTargetState(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())

	var headRoot [32]byte
	headRoot[0] = 1
	headState := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     1,
		LatestBlockHeader:        &types.BlockHeader{Slot: 1},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
	s.SetHead(headRoot)
	s.InsertState(headRoot, headState)

	attData := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Root: headRoot, Slot: 1},
		Target: &types.Checkpoint{Root: headRoot, Slot: 1},
		Source: &types.Checkpoint{Root: headRoot, Slot: 0},
	}
	dataRoot, _ := attData.HashTreeRoot()
	participants := types.NewBitlistSSZ(1)
	types.BitlistSet(participants, 0)
	s.NewPayloads.Push(dataRoot, attData, &types.SingleMessageAggregate{
		Participants: participants,
		Proof:        []byte{1},
	})

	snap := SnapshotInputs(s)
	if snap == nil {
		t.Fatal("snapshot is nil")
	}
	if snap.headState == nil || snap.headState.Slot != headState.Slot {
		t.Fatal("head state not captured")
	}
	if snap.newEntries[dataRoot] == nil {
		t.Fatal("new payload entry not captured")
	}
	if snap.targetStates[headRoot] == nil || snap.targetStates[headRoot].Slot != headState.Slot {
		t.Fatal("target state not captured")
	}
}

func TestSnapshotInputsReturnsNilWithoutWork(t *testing.T) {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	if snap := SnapshotInputs(s); snap != nil {
		t.Fatalf("snapshot=%v, want nil", snap)
	}
}
