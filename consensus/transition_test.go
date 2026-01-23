package consensus

import "testing"

func TestProcessSlot_FillsStateRoot(t *testing.T) {
	state := GenerateGenesis(0, 10)

	// Genesis header has zero state root
	if !state.LatestBlockHeader.StateRoot.IsZero() {
		t.Fatal("expected zero state root in genesis header")
	}

	// ProcessSlot should fill it
	newState, err := state.ProcessSlot()
	if err != nil {
		t.Fatalf("ProcessSlot error: %v", err)
	}

	if newState.LatestBlockHeader.StateRoot.IsZero() {
		t.Error("expected state root to be filled after ProcessSlot")
	}
}

func TestProcessSlot_NoOpIfStateRootSet(t *testing.T) {
	state := GenerateGenesis(0, 10)

	// Fill the state root first
	state, _ = state.ProcessSlot()
	originalRoot := state.LatestBlockHeader.StateRoot

	// ProcessSlot again should be no-op
	newState, err := state.ProcessSlot()
	if err != nil {
		t.Fatalf("ProcessSlot error: %v", err)
	}

	if newState.LatestBlockHeader.StateRoot != originalRoot {
		t.Error("state root should not change when already set")
	}
}

func TestProcessSlots(t *testing.T) {
	state := GenerateGenesis(0, 10)

	newState, err := state.ProcessSlots(5)
	if err != nil {
		t.Fatalf("ProcessSlots error: %v", err)
	}

	if newState.Slot != 5 {
		t.Errorf("Slot = %d, want 5", newState.Slot)
	}
}

func TestProcessSlots_TargetMustBeGreater(t *testing.T) {
	state := GenerateGenesis(0, 10)
	state.Slot = 5

	_, err := state.ProcessSlots(3)
	if err == nil {
		t.Error("expected error when target slot <= current slot")
	}
}

func TestStateCopy(t *testing.T) {
	state := GenerateGenesis(0, 10)
	state.HistoricalBlockHashes = append(state.HistoricalBlockHashes, Root{1, 2, 3})

	cp := state.Copy()

	// Modify original
	state.HistoricalBlockHashes[0] = Root{9, 9, 9}
	state.Slot = 999

	// Copy should be unaffected
	if cp.Slot != 0 {
		t.Error("copy Slot was modified")
	}
	if cp.HistoricalBlockHashes[0][0] != 1 {
		t.Error("copy HistoricalBlockHashes was modified")
	}
}

func TestBitlistHelpers(t *testing.T) {
	var bits []byte

	// Append bits at specific indices
	bits = appendBitAt(bits, 0, true)
	bits = appendBitAt(bits, 1, false)
	bits = appendBitAt(bits, 2, true)

	if !getBit(bits, 0) {
		t.Error("bit 0 should be true")
	}
	if getBit(bits, 1) {
		t.Error("bit 1 should be false")
	}
	if !getBit(bits, 2) {
		t.Error("bit 2 should be true")
	}

	// Set bit
	bits = setBit(bits, 1, true)
	if !getBit(bits, 1) {
		t.Error("bit 1 should be true after setBit")
	}

	bits = setBit(bits, 0, false)
	if getBit(bits, 0) {
		t.Error("bit 0 should be false after setBit")
	}
}

func TestGetBit_OutOfRange(t *testing.T) {
	bits := []byte{0xFF} // 8 bits

	// Should return false for out of range
	if getBit(bits, 100) {
		t.Error("out of range should return false")
	}
}

func TestProcessBlockHeader_ValidBlock(t *testing.T) {
	state := GenerateGenesis(0, 10)

	// Advance to slot 1
	state, _ = state.ProcessSlots(1)

	// Create a valid block
	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &Block{
		Slot:          1,
		ProposerIndex: 1, // slot 1 % 10 = 1
		ParentRoot:    parentRoot,
		StateRoot:     Root{},
		Body:          BlockBody{Attestations: []SignedVote{}},
	}

	newState, err := state.ProcessBlockHeader(block)
	if err != nil {
		t.Fatalf("ProcessBlockHeader error: %v", err)
	}

	// Should update latest block header
	if newState.LatestBlockHeader.Slot != 1 {
		t.Errorf("LatestBlockHeader.Slot = %d, want 1", newState.LatestBlockHeader.Slot)
	}

	// Should append to history
	if len(newState.HistoricalBlockHashes) != 1 {
		t.Errorf("HistoricalBlockHashes len = %d, want 1", len(newState.HistoricalBlockHashes))
	}

	// First block after genesis: should set justified/finalized roots
	if newState.LatestJustified.Root.IsZero() {
		t.Error("LatestJustified.Root should be set after first block")
	}
}

func TestProcessBlockHeader_InvalidProposer(t *testing.T) {
	state := GenerateGenesis(0, 10)
	state, _ = state.ProcessSlots(1)

	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()
	block := &Block{
		Slot:          1,
		ProposerIndex: 5, // wrong! should be 1
		ParentRoot:    parentRoot,
		StateRoot:     Root{},
		Body:          BlockBody{},
	}

	_, err := state.ProcessBlockHeader(block)
	if err == nil {
		t.Error("expected error for invalid proposer")
	}
}

func TestProcessBlockHeader_InvalidParentRoot(t *testing.T) {
	state := GenerateGenesis(0, 10)
	state, _ = state.ProcessSlots(1)

	block := &Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    Root{0xDE, 0xAD}, // wrong parent
		StateRoot:     Root{},
		Body:          BlockBody{},
	}

	_, err := state.ProcessBlockHeader(block)
	if err == nil {
		t.Error("expected error for invalid parent root")
	}
}
