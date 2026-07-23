package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func restoreTestStore() *store.ConsensusStore {
	return store.NewConsensusStore(storage.NewInMemoryBackend())
}

func minimalState() *types.State {
	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1},
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          &types.Checkpoint{},
		LatestFinalized:          &types.Checkpoint{},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

func insertProcessedBlock(s *store.ConsensusStore, root [32]byte, header *types.BlockHeader) {
	s.InsertBlockHeader(root, header)
	s.InsertState(root, minimalState())
}

func TestForkChoiceFromStoreRejectsMissingHeader(t *testing.T) {
	s := restoreTestStore()
	s.SetHead([32]byte{0x01})

	if _, err := ForkChoiceFromStore(s); err == nil {
		t.Fatal("expected missing head header error")
	}
}

func TestForkChoiceFromStoreFreshAnchorHasSingleNode(t *testing.T) {
	s := restoreTestStore()
	head := [32]byte{0x01}
	insertProcessedBlock(s, head, &types.BlockHeader{Slot: 0})
	s.SetHead(head)
	s.SetLatestJustified(&types.Checkpoint{Root: head, Slot: 0})

	fc, err := ForkChoiceFromStore(s)
	if err != nil {
		t.Fatalf("fork choice from store: %v", err)
	}
	if fc.Len() != 1 || fc.NodeIndex(head) != 0 {
		t.Fatalf("fresh store should anchor at head only: len=%d idx=%d", fc.Len(), fc.NodeIndex(head))
	}
}

func TestForkChoiceFromStoreReplaysChainToJustified(t *testing.T) {
	s := restoreTestStore()
	genesis := [32]byte{0xaa}
	blockA := [32]byte{0xbb}
	blockB := [32]byte{0xcc}
	blockC := [32]byte{0xdd}

	insertProcessedBlock(s, genesis, &types.BlockHeader{Slot: 0})
	insertProcessedBlock(s, blockA, &types.BlockHeader{Slot: 2, ParentRoot: genesis})
	insertProcessedBlock(s, blockB, &types.BlockHeader{Slot: 4, ParentRoot: blockA})
	insertProcessedBlock(s, blockC, &types.BlockHeader{Slot: 6, ParentRoot: blockB})
	s.SetHead(blockC)
	s.SetLatestJustified(&types.Checkpoint{Root: blockA, Slot: 2})

	fc, err := ForkChoiceFromStore(s)
	if err != nil {
		t.Fatalf("fork choice from store: %v", err)
	}
	if fc.NodeIndex(blockA) < 0 {
		t.Fatal("justified root missing from restored fork choice")
	}
	if head := fc.UpdateHead(blockA); head != blockC {
		t.Fatalf("head=%x, want stored leaf %x", head, blockC)
	}
}

func TestForkChoiceFromStoreFallsBackWhenJustifiedUnknown(t *testing.T) {
	s := restoreTestStore()
	head := [32]byte{0x02}
	insertProcessedBlock(s, head, &types.BlockHeader{Slot: 6, ParentRoot: [32]byte{0x03}})
	s.SetHead(head)
	s.SetLatestJustified(&types.Checkpoint{Root: [32]byte{0x09}, Slot: 2})

	fc, err := ForkChoiceFromStore(s)
	if err != nil {
		t.Fatalf("fork choice from store: %v", err)
	}
	if fc.Len() != 1 || fc.NodeIndex(head) != 0 {
		t.Fatalf("expected fallback anchor at head: len=%d idx=%d", fc.Len(), fc.NodeIndex(head))
	}
}

func TestForkChoiceFromStoreReplaysSiblingForks(t *testing.T) {
	s := restoreTestStore()
	justified := [32]byte{0xaa}
	staleB := [32]byte{0xbb}
	staleHead := [32]byte{0xcc}
	forkB := [32]byte{0xdd}
	forkTip := [32]byte{0xee}

	insertProcessedBlock(s, justified, &types.BlockHeader{Slot: 2})
	insertProcessedBlock(s, staleB, &types.BlockHeader{Slot: 4, ParentRoot: justified})
	insertProcessedBlock(s, staleHead, &types.BlockHeader{Slot: 6, ParentRoot: staleB})
	insertProcessedBlock(s, forkB, &types.BlockHeader{Slot: 3, ParentRoot: justified})
	insertProcessedBlock(s, forkTip, &types.BlockHeader{Slot: 5, ParentRoot: forkB})
	s.SetHead(staleHead)
	s.SetLatestJustified(&types.Checkpoint{Root: justified, Slot: 2})

	fc, err := ForkChoiceFromStore(s)
	if err != nil {
		t.Fatalf("fork choice from store: %v", err)
	}
	if fc.NodeIndex(forkTip) < 0 {
		t.Fatal("sibling fork missing from restored fork choice")
	}

	att := &types.AttestationData{Slot: 5, Head: &types.Checkpoint{Root: forkTip, Slot: 5}}
	for vid := uint64(0); vid < 3; vid++ {
		if !fc.SetKnownVote(vid, forkTip, 5, att) {
			t.Fatalf("vote for fork tip rejected for validator %d", vid)
		}
	}
	if head := fc.UpdateHead(justified); head != forkTip {
		t.Fatalf("head=%x, want weighted fork tip %x", head, forkTip)
	}
}

func TestForkChoiceFromStoreSkipsUnprocessedHeaders(t *testing.T) {
	s := restoreTestStore()
	justified := [32]byte{0xaa}
	processed := [32]byte{0xbb}
	unverified := [32]byte{0xcc}

	insertProcessedBlock(s, justified, &types.BlockHeader{Slot: 2})
	insertProcessedBlock(s, processed, &types.BlockHeader{Slot: 4, ParentRoot: justified})
	// Header persisted for a pending block that was never verified: no state.
	s.InsertBlockHeader(unverified, &types.BlockHeader{Slot: 5, ParentRoot: justified})
	s.SetHead(processed)
	s.SetLatestJustified(&types.Checkpoint{Root: justified, Slot: 2})

	fc, err := ForkChoiceFromStore(s)
	if err != nil {
		t.Fatalf("fork choice from store: %v", err)
	}
	if fc.NodeIndex(unverified) >= 0 {
		t.Fatal("unverified header must not enter restored fork choice")
	}
	if head := fc.UpdateHead(justified); head != processed {
		t.Fatalf("head=%x, want processed block %x", head, processed)
	}
}
