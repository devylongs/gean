package types

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// These tests verify hash_tree_root correctness against known test vectors.

// chunk pads hex payload to 32 bytes (right-padded with zeros).
func chunk(hexPayload string) [32]byte {
	var c [32]byte
	b, _ := hex.DecodeString(hexPayload)
	copy(c[:], b)
	return c
}

// h computes SHA256(a || b) for two 32-byte chunks.
func h(a, b [32]byte) [32]byte {
	var combined [64]byte
	copy(combined[:32], a[:])
	copy(combined[32:], b[:])
	return sha256.Sum256(combined[:])
}

func rootHex(root [32]byte) string {
	return hex.EncodeToString(root[:])
}

// Test uint64 hash_tree_root: little-endian padded to 32 bytes.
func TestHashTreeRootUint64Compliance(t *testing.T) {
	// ChainConfig has a single uint64 field, so its root = merkleize([padded_uint64])
	// For a 1-field container, merkleize of 1 chunk = the chunk itself.
	cfg := &ChainConfig{GenesisTime: 0}
	root, err := cfg.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	// genesis_time=0 → chunk is all zeros → root should be all zeros
	if root != [32]byte{} {
		t.Fatalf("ChainConfig(0) root should be zero, got %s", rootHex(root))
	}

	cfg2 := &ChainConfig{GenesisTime: 1}
	root2, err := cfg2.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	// genesis_time=1 → LE bytes = 0x0100000000000000, padded to 32
	expected := chunk("0100000000000000")
	if root2 != expected {
		t.Fatalf("ChainConfig(1) expected %s, got %s", rootHex(expected), rootHex(root2))
	}
}

// Test Checkpoint hash_tree_root: container with 2 fields (root, slot).
// Merkle tree: h(field0_chunk, field1_chunk)
func TestCheckpointHashTreeRootCompliance(t *testing.T) {
	cp := &Checkpoint{
		Root: [32]byte{0xab},
		Slot: 0,
	}
	root, err := cp.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	// Field 0: root = [0xab, 0, 0, ..., 0] (already 32 bytes, identity)
	// Field 1: slot = 0 → chunk of all zeros
	// Merkle: h(field0, field1) since 2 fields → depth 1
	field0 := chunk("ab")
	field1 := chunk("")
	expected := h(field0, field1)
	if root != expected {
		t.Fatalf("Checkpoint(0xab,0) expected %s, got %s", rootHex(expected), rootHex(root))
	}
}

// Test Checkpoint with non-zero slot.
func TestCheckpointHashTreeRootWithSlot(t *testing.T) {
	cp := &Checkpoint{
		Root: [32]byte{},
		Slot: 42,
	}
	root, err := cp.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	field0 := chunk("")                 // root = zero
	field1 := chunk("2a00000000000000") // slot = 42 LE
	expected := h(field0, field1)
	if root != expected {
		t.Fatalf("Checkpoint(0,42) expected %s, got %s", rootHex(expected), rootHex(root))
	}
}

// Test BlockHeader hash_tree_root: 5-field container.
// 5 fields → pad to 8 → depth 3 merkle tree.
func TestBlockHeaderHashTreeRootCompliance(t *testing.T) {
	hdr := &BlockHeader{
		Slot:          1,
		ProposerIndex: 0,
		ParentRoot:    [32]byte{0x01},
		StateRoot:     [32]byte{0x02},
		BodyRoot:      [32]byte{0x03},
	}
	root, err := hdr.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	// 5 fields padded to 8:
	f0 := chunk("0100000000000000") // slot=1 LE
	f1 := chunk("")                 // proposer_index=0
	f2 := chunk("01")               // parent_root
	f3 := chunk("02")               // state_root
	f4 := chunk("03")               // body_root
	f5 := chunk("")                 // padding
	f6 := chunk("")                 // padding
	f7 := chunk("")                 // padding

	expected := h(
		h(h(f0, f1), h(f2, f3)),
		h(h(f4, f5), h(f6, f7)),
	)
	if root != expected {
		t.Fatalf("BlockHeader expected %s, got %s", rootHex(expected), rootHex(root))
	}
}

// Test Validator hash_tree_root: 3-field container.
// pubkey (32 bytes) → exactly 1 chunk, so each pubkey field root is the chunk itself.
func TestValidatorHashTreeRootCompliance(t *testing.T) {
	v := &Validator{
		AttestationPubkey: [32]byte{},
		ProposalPubkey:    [32]byte{},
		Index:             0,
	}
	root, err := v.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	// 3-field container: attestation_pubkey, proposal_pubkey, index
	// 3 fields → pad to 4 → depth 2 merkle tree
	attPubkeyRoot := chunk("")  // 32-byte vector = single zero chunk, no merkleize
	propPubkeyRoot := chunk("") // same (both zero)
	indexRoot := chunk("")
	// h(h(att_pubkey, prop_pubkey), h(index, zero))
	expected := h(h(attPubkeyRoot, propPubkeyRoot), h(indexRoot, chunk("")))
	if root != expected {
		t.Fatalf("Validator(zeros) expected %s, got %s", rootHex(expected), rootHex(root))
	}
}

// Test AttestationData hash_tree_root: 4-field container.
// 4 fields → pad to 4 → depth 2 merkle tree.
func TestAttestationDataHashTreeRootCompliance(t *testing.T) {
	d := &AttestationData{
		Slot:   0,
		Head:   &Checkpoint{Root: [32]byte{}, Slot: 0},
		Target: &Checkpoint{Root: [32]byte{}, Slot: 0},
		Source: &Checkpoint{Root: [32]byte{}, Slot: 0},
	}
	root, err := d.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	// All fields are zero → all field roots are h(zeros, zeros) or zeros
	slotRoot := chunk("")
	cpRoot := h(chunk(""), chunk("")) // Checkpoint(zero, 0)
	expected := h(
		h(slotRoot, cpRoot),
		h(cpRoot, cpRoot),
	)
	if root != expected {
		t.Fatalf("AttestationData(zeros) expected %s, got %s", rootHex(expected), rootHex(root))
	}
}

// Test determinism: same input always produces same root.
func TestHashTreeRootDeterminism(t *testing.T) {
	s := &State{
		Config:                   &ChainConfig{GenesisTime: 1704085200},
		Slot:                     100,
		LatestBlockHeader:        &BlockHeader{Slot: 99, ProposerIndex: 2, ParentRoot: [32]byte{0xff}},
		LatestJustified:          &Checkpoint{Root: [32]byte{0xaa}, Slot: 50},
		LatestFinalized:          &Checkpoint{Root: [32]byte{0xbb}, Slot: 30},
		HistoricalBlockHashes:    [][]byte{make([]byte, 32), make([]byte, 32)},
		JustifiedSlots:           NewBitlistSSZ(100),
		Validators:               []*Validator{{AttestationPubkey: [32]byte{1}, Index: 0}, {AttestationPubkey: [32]byte{2}, Index: 1}},
		JustificationsRoots:      [][]byte{make([]byte, 32)},
		JustificationsValidators: NewBitlistSSZ(10),
	}
	root1, err := s.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	root2, err := s.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root1 != root2 {
		t.Fatalf("hash_tree_root not deterministic: %s vs %s", rootHex(root1), rootHex(root2))
	}
	if root1 == [32]byte{} {
		t.Fatal("state root should not be zero for non-trivial state")
	}
}

// Test SSZ encoding size matches expected fixed sizes.
func TestSSZSizeCompliance(t *testing.T) {
	// Checkpoint: 32 (root) + 8 (slot) = 40 bytes
	cp := &Checkpoint{}
	if cp.SizeSSZ() != 40 {
		t.Fatalf("Checkpoint size: expected 40, got %d", cp.SizeSSZ())
	}

	// ChainConfig: 8 bytes
	cfg := &ChainConfig{}
	if cfg.SizeSSZ() != 8 {
		t.Fatalf("ChainConfig size: expected 8, got %d", cfg.SizeSSZ())
	}

	// Validator: 32 (attestation_pubkey) + 32 (proposal_pubkey) + 8 (index) = 72 bytes
	v := &Validator{}
	if v.SizeSSZ() != 72 {
		t.Fatalf("Validator size: expected 72, got %d", v.SizeSSZ())
	}

	// BlockHeader: 8+8+32+32+32 = 112 bytes
	hdr := &BlockHeader{}
	if hdr.SizeSSZ() != 112 {
		t.Fatalf("BlockHeader size: expected 112, got %d", hdr.SizeSSZ())
	}

	// AttestationData: 8 + 40 + 40 + 40 = 128 bytes
	ad := &AttestationData{Head: &Checkpoint{}, Target: &Checkpoint{}, Source: &Checkpoint{}}
	if ad.SizeSSZ() != 128 {
		t.Fatalf("AttestationData size: expected 128, got %d", ad.SizeSSZ())
	}
}
