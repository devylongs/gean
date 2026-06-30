package attestation_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/types"
)

func TestProduceAttestationDataNilWhenHeadStateMissing(t *testing.T) {
	s := makeValidationStore()
	s.SetHead([32]byte{0xAA})
	if got := attestation.ProduceAttestationData(s, 5); got != nil {
		t.Fatalf("expected nil when head state missing, got %+v", got)
	}
}

// At genesis the head state's justified root is the zero placeholder; the
// produced source resolves it to the head root so it names a real block.
func TestProduceAttestationDataGenesisSource(t *testing.T) {
	s := makeValidationStore()
	head := [32]byte{0xAA}

	s.SetHead(head)
	s.SetSafeTarget(head)
	s.SetLatestJustified(&types.Checkpoint{Slot: 0})
	s.SetLatestFinalized(&types.Checkpoint{Slot: 0})
	s.InsertBlockHeader(head, &types.BlockHeader{Slot: 0})
	s.InsertState(head, &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		LatestBlockHeader:        &types.BlockHeader{Slot: 0}, // genesis
		LatestJustified:          &types.Checkpoint{Slot: 0},  // zero root placeholder
		LatestFinalized:          &types.Checkpoint{},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	})

	data := attestation.ProduceAttestationData(s, 5)
	if data == nil {
		t.Fatal("expected attestation data")
	}
	if data.Source.Root != head {
		t.Fatalf("genesis source root = %x, want head %x", data.Source.Root, head)
	}
	if data.Source.Slot != 0 {
		t.Fatalf("source slot = %d, want justified slot 0", data.Source.Slot)
	}
	if data.Slot != 5 {
		t.Fatalf("data slot = %d, want 5", data.Slot)
	}
}

// The source is the head chain's own justified checkpoint, not the store's
// global latest justified — the two can diverge when the store advanced
// justification on a fork the head never extended.
func TestProduceAttestationDataSourceFromHeadStateNotStore(t *testing.T) {
	s := makeValidationStore()
	head := [32]byte{0xAA}
	headJustified := [32]byte{0xBB}
	storeJustified := [32]byte{0xCC}

	s.SetHead(head)
	s.SetSafeTarget(head)
	// Store's global justified differs from the head state's justified.
	s.SetLatestJustified(&types.Checkpoint{Root: storeJustified, Slot: 4})
	s.SetLatestFinalized(&types.Checkpoint{Slot: 6})
	s.InsertBlockHeader(head, &types.BlockHeader{Slot: 6})
	s.InsertState(head, &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		LatestBlockHeader:        &types.BlockHeader{Slot: 6},
		LatestJustified:          &types.Checkpoint{Root: headJustified, Slot: 4},
		LatestFinalized:          &types.Checkpoint{Slot: 6},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	})

	data := attestation.ProduceAttestationData(s, 6)
	if data == nil {
		t.Fatal("expected attestation data")
	}
	if data.Source.Root != headJustified {
		t.Fatalf("source root = %x, want head-state justified %x (not store %x)", data.Source.Root, headJustified, storeJustified)
	}
}

// Post-genesis the source is the stored justified checkpoint unchanged.
func TestProduceAttestationDataNormalSource(t *testing.T) {
	s := makeValidationStore()
	head := [32]byte{0xAA}
	justifiedRoot := [32]byte{0xBB}

	s.SetHead(head)
	s.SetSafeTarget(head)
	s.SetLatestJustified(&types.Checkpoint{Root: justifiedRoot, Slot: 4})
	s.SetLatestFinalized(&types.Checkpoint{Slot: 6})
	s.InsertBlockHeader(head, &types.BlockHeader{Slot: 6})
	s.InsertState(head, &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		LatestBlockHeader:        &types.BlockHeader{Slot: 6}, // non-genesis
		LatestJustified:          &types.Checkpoint{Root: justifiedRoot, Slot: 4},
		LatestFinalized:          &types.Checkpoint{Slot: 6},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	})

	data := attestation.ProduceAttestationData(s, 6)
	if data == nil {
		t.Fatal("expected attestation data")
	}
	if data.Source.Root != justifiedRoot {
		t.Fatalf("source root = %x, want justified %x", data.Source.Root, justifiedRoot)
	}
	if data.Head.Root != head {
		t.Fatalf("head root = %x, want %x", data.Head.Root, head)
	}
}
