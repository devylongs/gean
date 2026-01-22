package transition

import (
	"testing"

	"github.com/devylongs/gean/types"
)

// createTestState creates a minimal genesis state for testing.
func createTestState() *types.State {
	emptyBody := types.BlockBody{Attestations: []types.SignedVote{}}
	bodyRoot, _ := emptyBody.HashTreeRoot()

	return &types.State{
		Config: types.Config{
			NumValidators: 4,
			GenesisTime:   1700000000,
		},
		Slot: 0,
		LatestBlockHeader: types.BlockHeader{
			Slot:          0,
			ProposerIndex: 0,
			ParentRoot:    types.Root{},
			StateRoot:     types.Root{},
			BodyRoot:      bodyRoot,
		},
		LatestJustified:          types.Checkpoint{Root: types.Root{}, Slot: 0},
		LatestFinalized:          types.Checkpoint{Root: types.Root{}, Slot: 0},
		HistoricalBlockHashes:    []types.Root{},
		JustifiedSlots:           []byte{0x01}, // Empty bitlist (just sentinel)
		JustificationsRoots:      []types.Root{},
		JustificationsValidators: []byte{0x01}, // Empty bitlist (just sentinel)
	}
}

func TestBitlistLen(t *testing.T) {
	tests := []struct {
		name     string
		bitlist  []byte
		expected int
	}{
		{"empty (just sentinel)", []byte{0x01}, 0},
		{"1 bit false", []byte{0x02}, 1},
		{"1 bit true", []byte{0x03}, 1},
		{"2 bits", []byte{0x04}, 2},
		{"3 bits", []byte{0x08}, 3},
		{"8 bits", []byte{0x00, 0x01}, 8},
		{"9 bits", []byte{0x00, 0x02}, 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bitlistLen(tt.bitlist)
			if got != tt.expected {
				t.Errorf("bitlistLen(%v) = %d, want %d", tt.bitlist, got, tt.expected)
			}
		})
	}
}

func TestGetBit(t *testing.T) {
	// Bitlist: [1, 0, 1] with sentinel -> 0b1101 = 0x0D
	bitlist := []byte{0x0D} // bits: 1, 0, 1, sentinel

	tests := []struct {
		index    int
		expected bool
	}{
		{0, true},
		{1, false},
		{2, true},
		{3, false}, // Out of bounds
	}

	for _, tt := range tests {
		got := getBit(bitlist, tt.index)
		if got != tt.expected {
			t.Errorf("getBit(bitlist, %d) = %v, want %v", tt.index, got, tt.expected)
		}
	}
}

func TestAppendBit(t *testing.T) {
	// Start with empty bitlist.
	bitlist := []byte{0x01}

	// Append true -> [true] with sentinel = 0b11 = 0x03
	bitlist = appendBit(bitlist, true)
	if bitlistLen(bitlist) != 1 {
		t.Errorf("after append true: len = %d, want 1", bitlistLen(bitlist))
	}
	if !getBit(bitlist, 0) {
		t.Errorf("after append true: bit 0 should be true")
	}

	// Append false -> [true, false] with sentinel = 0b101 = 0x05
	bitlist = appendBit(bitlist, false)
	if bitlistLen(bitlist) != 2 {
		t.Errorf("after append false: len = %d, want 2", bitlistLen(bitlist))
	}
	if !getBit(bitlist, 0) {
		t.Errorf("after append false: bit 0 should still be true")
	}
	if getBit(bitlist, 1) {
		t.Errorf("after append false: bit 1 should be false")
	}

	// Append true -> [true, false, true] with sentinel = 0b1101 = 0x0D
	bitlist = appendBit(bitlist, true)
	if bitlistLen(bitlist) != 3 {
		t.Errorf("after second append true: len = %d, want 3", bitlistLen(bitlist))
	}
	if !getBit(bitlist, 2) {
		t.Errorf("after second append true: bit 2 should be true")
	}
}

func TestProcessSlot_EmptyStateRoot(t *testing.T) {
	state := createTestState()

	// Initially, state root is zero.
	if !state.LatestBlockHeader.StateRoot.IsZero() {
		t.Fatal("initial state root should be zero")
	}

	// Process slot should fill in the state root.
	newState, err := ProcessSlot(state)
	if err != nil {
		t.Fatalf("ProcessSlot failed: %v", err)
	}

	if newState.LatestBlockHeader.StateRoot.IsZero() {
		t.Error("state root should be filled after ProcessSlot")
	}
}

func TestProcessSlot_NonEmptyStateRoot(t *testing.T) {
	state := createTestState()
	state.LatestBlockHeader.StateRoot = types.Root{0x01, 0x02, 0x03}

	// Process slot should not change an already-set state root.
	newState, err := ProcessSlot(state)
	if err != nil {
		t.Fatalf("ProcessSlot failed: %v", err)
	}

	if newState.LatestBlockHeader.StateRoot != state.LatestBlockHeader.StateRoot {
		t.Error("state root should not change when already set")
	}
}

func TestProcessSlots(t *testing.T) {
	state := createTestState()
	state.Slot = 0

	// Advance from slot 0 to slot 5.
	newState, err := ProcessSlots(state, 5)
	if err != nil {
		t.Fatalf("ProcessSlots failed: %v", err)
	}

	if newState.Slot != 5 {
		t.Errorf("slot = %d, want 5", newState.Slot)
	}
}

func TestProcessSlots_NotInFuture(t *testing.T) {
	state := createTestState()
	state.Slot = 5

	// Target slot must be in the future.
	_, err := ProcessSlots(state, 5)
	if err != ErrSlotNotInFuture {
		t.Errorf("expected ErrSlotNotInFuture, got %v", err)
	}

	_, err = ProcessSlots(state, 3)
	if err != ErrSlotNotInFuture {
		t.Errorf("expected ErrSlotNotInFuture for past slot, got %v", err)
	}
}

func TestProcessBlockHeader_ValidBlock(t *testing.T) {
	state := createTestState()

	// First, fill in the state root via ProcessSlot and advance to slot 1.
	state, _ = ProcessSlot(state)
	state.Slot = 1

	// Get parent root (hash of latest block header).
	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()

	// Create a valid block for slot 1.
	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1, // slot 1 % 4 validators = 1
		ParentRoot:    parentRoot,
		StateRoot:     types.Root{}, // Will be validated later
		Body:          types.BlockBody{Attestations: []types.SignedVote{}},
	}

	newState, err := ProcessBlockHeader(state, block)
	if err != nil {
		t.Fatalf("ProcessBlockHeader failed: %v", err)
	}

	// Check that latest block header was updated.
	if newState.LatestBlockHeader.Slot != 1 {
		t.Errorf("latest block header slot = %d, want 1", newState.LatestBlockHeader.Slot)
	}

	// Check that historical hashes was updated.
	if len(newState.HistoricalBlockHashes) != 1 {
		t.Errorf("historical block hashes len = %d, want 1", len(newState.HistoricalBlockHashes))
	}

	// First block after genesis should set justified/finalized roots.
	if newState.LatestJustified.Root != parentRoot {
		t.Error("latest justified root should be set to parent root for first block")
	}
	if newState.LatestFinalized.Root != parentRoot {
		t.Error("latest finalized root should be set to parent root for first block")
	}
}

func TestProcessBlockHeader_SlotMismatch(t *testing.T) {
	state := createTestState()
	state.Slot = 5

	block := &types.Block{
		Slot:          3, // Mismatch
		ProposerIndex: 3,
		ParentRoot:    types.Root{},
		Body:          types.BlockBody{},
	}

	_, err := ProcessBlockHeader(state, block)
	if err != ErrSlotMismatch {
		t.Errorf("expected ErrSlotMismatch, got %v", err)
	}
}

func TestProcessBlockHeader_InvalidProposer(t *testing.T) {
	state := createTestState()
	state, _ = ProcessSlot(state)
	state.Slot = 1
	parentRoot, _ := state.LatestBlockHeader.HashTreeRoot()

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 0, // Wrong! Should be 1 for slot 1 with 4 validators.
		ParentRoot:    parentRoot,
		Body:          types.BlockBody{},
	}

	_, err := ProcessBlockHeader(state, block)
	if err != ErrInvalidProposer {
		t.Errorf("expected ErrInvalidProposer, got %v", err)
	}
}

func TestProcessBlockHeader_ParentRootMismatch(t *testing.T) {
	state := createTestState()
	state, _ = ProcessSlot(state)
	state.Slot = 1

	block := &types.Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    types.Root{0xff}, // Wrong parent root
		Body:          types.BlockBody{},
	}

	_, err := ProcessBlockHeader(state, block)
	if err != ErrParentRootMismatch {
		t.Errorf("expected ErrParentRootMismatch, got %v", err)
	}
}

func TestCopyState(t *testing.T) {
	state := createTestState()
	state.HistoricalBlockHashes = []types.Root{{0x01}, {0x02}}

	copied := copyState(state)

	// Modify the copy.
	copied.Slot = 999
	copied.HistoricalBlockHashes[0] = types.Root{0xff}

	// Original should be unchanged.
	if state.Slot == 999 {
		t.Error("original state slot was modified")
	}
	if state.HistoricalBlockHashes[0] == (types.Root{0xff}) {
		t.Error("original state historical hashes was modified")
	}
}
