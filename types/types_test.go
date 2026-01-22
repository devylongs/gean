package types

import (
	"bytes"
	"testing"
)

func TestSlotToTime(t *testing.T) {
	genesis := uint64(1700000000)
	if SlotToTime(0, genesis) != 1700000000 {
		t.Error("slot 0")
	}
	if SlotToTime(1, genesis) != 1700000004 {
		t.Error("slot 1")
	}
	if SlotToTime(100, genesis) != 1700000400 {
		t.Error("slot 100")
	}
}

func TestTimeToSlot(t *testing.T) {
	genesis := uint64(1700000000)
	if TimeToSlot(1700000000, genesis) != 0 {
		t.Error("time at genesis")
	}
	if TimeToSlot(1700000004, genesis) != 1 {
		t.Error("time +4s")
	}
	if TimeToSlot(1699999999, genesis) != 0 {
		t.Error("time before genesis")
	}
}

func TestRootIsZero(t *testing.T) {
	var zero Root
	if !zero.IsZero() {
		t.Error("zero root")
	}
	if (Root{1}).IsZero() {
		t.Error("non-zero root")
	}
}

func TestCheckpointSSZRoundTrip(t *testing.T) {
	original := &Checkpoint{
		Root: Root{0xab, 0xcd, 0xef},
		Slot: 100,
	}

	// Serialize
	data, err := original.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	// Check expected size (32 bytes for Root + 8 bytes for Slot = 40 bytes)
	if len(data) != 40 {
		t.Errorf("expected 40 bytes, got %d", len(data))
	}

	// Deserialize
	decoded := &Checkpoint{}
	err = decoded.UnmarshalSSZ(data)
	if err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}

	// Compare
	if !bytes.Equal(decoded.Root[:], original.Root[:]) {
		t.Errorf("Root mismatch: got %x, want %x", decoded.Root, original.Root)
	}
	if decoded.Slot != original.Slot {
		t.Errorf("Slot mismatch: got %d, want %d", decoded.Slot, original.Slot)
	}
}

func TestCheckpointHashTreeRoot(t *testing.T) {
	checkpoint := &Checkpoint{
		Root: Root{0xab, 0xcd, 0xef},
		Slot: 100,
	}

	root, err := checkpoint.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot failed: %v", err)
	}

	// Root should be 32 bytes and non-zero
	if len(root) != 32 {
		t.Errorf("expected 32 byte root, got %d", len(root))
	}

	var zeroRoot [32]byte
	if root == zeroRoot {
		t.Error("hash tree root should not be zero")
	}

	// Same input should produce same root
	root2, _ := checkpoint.HashTreeRoot()
	if root != root2 {
		t.Error("hash tree root should be deterministic")
	}
}

func TestConfigSSZRoundTrip(t *testing.T) {
	original := &Config{
		NumValidators: 100,
		GenesisTime:   1700000000,
	}

	data, err := original.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}
	// 8 bytes for NumValidators + 8 bytes for GenesisTime = 16 bytes
	if len(data) != 16 {
		t.Errorf("expected 16 bytes, got %d", len(data))
	}

	decoded := &Config{}
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}
	if decoded.NumValidators != original.NumValidators {
		t.Errorf("NumValidators mismatch: got %d, want %d", decoded.NumValidators, original.NumValidators)
	}
	if decoded.GenesisTime != original.GenesisTime {
		t.Errorf("GenesisTime mismatch: got %d, want %d", decoded.GenesisTime, original.GenesisTime)
	}
}

func TestVoteSSZRoundTrip(t *testing.T) {
	original := &Vote{
		ValidatorID: 42,
		Slot:        100,
		Head:        Checkpoint{Root: Root{0x01}, Slot: 99},
		Target:      Checkpoint{Root: Root{0x02}, Slot: 98},
		Source:      Checkpoint{Root: Root{0x03}, Slot: 97},
	}

	data, err := original.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}
	// 8 (ValidatorID) + 8 (Slot) + 40*3 (Head, Target, Source) = 136 bytes
	if len(data) != 136 {
		t.Errorf("expected 136 bytes, got %d", len(data))
	}

	decoded := &Vote{}
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}
	if decoded.ValidatorID != original.ValidatorID {
		t.Errorf("ValidatorID mismatch: got %d, want %d", decoded.ValidatorID, original.ValidatorID)
	}
	if decoded.Slot != original.Slot {
		t.Errorf("Slot mismatch: got %d, want %d", decoded.Slot, original.Slot)
	}
	if decoded.Head.Slot != original.Head.Slot {
		t.Errorf("Head.Slot mismatch: got %d, want %d", decoded.Head.Slot, original.Head.Slot)
	}
}

func TestSignedVoteSSZRoundTrip(t *testing.T) {
	original := &SignedVote{
		Data: Vote{
			ValidatorID: 42,
			Slot:        100,
			Head:        Checkpoint{Root: Root{0x01}, Slot: 99},
			Target:      Checkpoint{Root: Root{0x02}, Slot: 98},
			Source:      Checkpoint{Root: Root{0x03}, Slot: 97},
		},
		Signature: Bytes32{0xaa, 0xbb, 0xcc},
	}

	data, err := original.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}
	// 136 (Vote) + 32 (Signature) = 168 bytes
	if len(data) != 168 {
		t.Errorf("expected 168 bytes, got %d", len(data))
	}

	decoded := &SignedVote{}
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}
	if decoded.Data.ValidatorID != original.Data.ValidatorID {
		t.Errorf("Data.ValidatorID mismatch")
	}
	if !bytes.Equal(decoded.Signature[:], original.Signature[:]) {
		t.Errorf("Signature mismatch")
	}
}

func TestBlockSSZRoundTrip(t *testing.T) {
	original := &Block{
		Slot:          100,
		ProposerIndex: 5,
		ParentRoot:    Root{0xaa},
		StateRoot:     Root{0xbb},
		Body: BlockBody{
			Attestations: []SignedVote{},
		},
	}

	data, err := original.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	decoded := &Block{}
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}
	if decoded.Slot != original.Slot {
		t.Errorf("Slot mismatch")
	}
	if decoded.ProposerIndex != original.ProposerIndex {
		t.Errorf("ProposerIndex mismatch")
	}
}

func TestSignedBlockSSZRoundTrip(t *testing.T) {
	original := &SignedBlock{
		Message: Block{
			Slot:          100,
			ProposerIndex: 5,
			ParentRoot:    Root{0xaa},
			StateRoot:     Root{0xbb},
			Body: BlockBody{
				Attestations: []SignedVote{},
			},
		},
		Signature: Bytes32{0xdd, 0xee, 0xff},
	}

	data, err := original.MarshalSSZ()
	if err != nil {
		t.Fatalf("MarshalSSZ failed: %v", err)
	}

	decoded := &SignedBlock{}
	if err := decoded.UnmarshalSSZ(data); err != nil {
		t.Fatalf("UnmarshalSSZ failed: %v", err)
	}
	if decoded.Message.Slot != original.Message.Slot {
		t.Errorf("Message.Slot mismatch")
	}
	if !bytes.Equal(decoded.Signature[:], original.Signature[:]) {
		t.Errorf("Signature mismatch")
	}
}

func TestBlockHeaderHashTreeRoot(t *testing.T) {
	header := &BlockHeader{
		Slot:          100,
		ProposerIndex: 5,
		ParentRoot:    Root{0xaa},
		StateRoot:     Root{0xbb},
		BodyRoot:      Root{0xcc},
	}

	root, err := header.HashTreeRoot()
	if err != nil {
		t.Fatalf("HashTreeRoot failed: %v", err)
	}

	var zeroRoot [32]byte
	if root == zeroRoot {
		t.Error("hash tree root should not be zero")
	}

	root2, _ := header.HashTreeRoot()
	if root != root2 {
		t.Error("hash tree root should be deterministic")
	}
}

func TestSlotIsJustifiableAfter(t *testing.T) {
	tests := []struct {
		slot      Slot
		finalized Slot
		want      bool
		desc      string
	}{
		// Rule 1: delta <= 5
		{0, 0, true, "delta=0"},
		{1, 0, true, "delta=1"},
		{5, 0, true, "delta=5"},

		// Rule 2: Perfect squares (delta = 9, 16, 25, 36, 49, 64, 81, 100)
		{9, 0, true, "delta=9 (3^2)"},
		{16, 0, true, "delta=16 (4^2)"},
		{25, 0, true, "delta=25 (5^2)"},
		{36, 0, true, "delta=36 (6^2)"},
		{100, 0, true, "delta=100 (10^2)"},

		// Rule 3: Pronic numbers (x*(x+1): 6, 12, 20, 30, 42, 56, 72, 90, 110)
		{6, 0, true, "delta=6 (2*3)"},
		{12, 0, true, "delta=12 (3*4)"},
		{20, 0, true, "delta=20 (4*5)"},
		{30, 0, true, "delta=30 (5*6)"},
		{42, 0, true, "delta=42 (6*7)"},
		{56, 0, true, "delta=56 (7*8)"},
		{72, 0, true, "delta=72 (8*9)"},
		{90, 0, true, "delta=90 (9*10)"},
		{110, 0, true, "delta=110 (10*11)"},

		// Non-justifiable slots
		{7, 0, false, "delta=7 (not square, not pronic)"},
		{8, 0, false, "delta=8 (not square, not pronic)"},
		{10, 0, false, "delta=10 (not square, not pronic)"},
		{11, 0, false, "delta=11 (not square, not pronic)"},
		{13, 0, false, "delta=13 (not square, not pronic)"},
		{15, 0, false, "delta=15 (not square, not pronic)"},
		{17, 0, false, "delta=17 (not square, not pronic)"},

		// With non-zero finalized slot
		{10, 5, true, "delta=5 with finalized=5"},
		{14, 5, true, "delta=9 (3^2) with finalized=5"},
		{11, 5, true, "delta=6 (2*3) with finalized=5"},
		{12, 5, false, "delta=7 with finalized=5"},

		// Slot before finalized (should return false)
		{5, 10, false, "slot before finalized"},
	}

	for _, tt := range tests {
		got := tt.slot.IsJustifiableAfter(tt.finalized)
		if got != tt.want {
			t.Errorf("%s: Slot(%d).IsJustifiableAfter(%d) = %v, want %v",
				tt.desc, tt.slot, tt.finalized, got, tt.want)
		}
	}
}

func TestIsProposer(t *testing.T) {
	tests := []struct {
		validatorIndex ValidatorIndex
		slot           Slot
		numValidators  uint64
		want           bool
		desc           string
	}{
		// Basic round-robin
		{0, 0, 4, true, "validator 0 at slot 0"},
		{1, 1, 4, true, "validator 1 at slot 1"},
		{2, 2, 4, true, "validator 2 at slot 2"},
		{3, 3, 4, true, "validator 3 at slot 3"},
		{0, 4, 4, true, "validator 0 at slot 4 (wraps)"},
		{1, 5, 4, true, "validator 1 at slot 5 (wraps)"},

		// Non-proposer cases
		{0, 1, 4, false, "validator 0 at slot 1"},
		{1, 0, 4, false, "validator 1 at slot 0"},
		{2, 5, 4, false, "validator 2 at slot 5"},

		// Edge cases
		{0, 0, 1, true, "single validator always proposes"},
		{0, 100, 1, true, "single validator at slot 100"},
		{0, 0, 0, false, "zero validators"},
	}

	for _, tt := range tests {
		got := IsProposer(tt.validatorIndex, tt.slot, tt.numValidators)
		if got != tt.want {
			t.Errorf("%s: IsProposer(%d, %d, %d) = %v, want %v",
				tt.desc, tt.validatorIndex, tt.slot, tt.numValidators, got, tt.want)
		}
	}
}

func TestIsPerfectSquare(t *testing.T) {
	squares := []uint64{0, 1, 4, 9, 16, 25, 36, 49, 64, 81, 100, 121, 144}
	for _, n := range squares {
		if !isPerfectSquare(n) {
			t.Errorf("isPerfectSquare(%d) = false, want true", n)
		}
	}

	nonSquares := []uint64{2, 3, 5, 6, 7, 8, 10, 11, 12, 13, 14, 15, 17, 99, 101}
	for _, n := range nonSquares {
		if isPerfectSquare(n) {
			t.Errorf("isPerfectSquare(%d) = true, want false", n)
		}
	}
}
