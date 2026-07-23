package types

import (
	"testing"
)

func TestCheckpointSSZRoundtrip(t *testing.T) {
	c := &Checkpoint{Root: [32]byte{0xab, 0xcd}, Slot: 42}
	data, err := c.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}
	c2 := &Checkpoint{}
	if err := c2.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}
	if c.Root != c2.Root || c.Slot != c2.Slot {
		t.Fatal("roundtrip mismatch")
	}
}

func TestCheckpointHashTreeRoot(t *testing.T) {
	c := &Checkpoint{Root: [32]byte{1}, Slot: 100}
	root, err := c.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root == [32]byte{} {
		t.Fatal("root should not be zero")
	}
	// Deterministic
	root2, _ := c.HashTreeRoot()
	if root != root2 {
		t.Fatal("hash_tree_root not deterministic")
	}
}

func TestBlockHeaderSSZRoundtrip(t *testing.T) {
	h := &BlockHeader{
		Slot: 5, ProposerIndex: 2,
		ParentRoot: [32]byte{1}, StateRoot: [32]byte{2}, BodyRoot: [32]byte{3},
	}
	data, err := h.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}
	h2 := &BlockHeader{}
	if err := h2.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}
	if h.Slot != h2.Slot || h.ProposerIndex != h2.ProposerIndex ||
		h.ParentRoot != h2.ParentRoot || h.StateRoot != h2.StateRoot || h.BodyRoot != h2.BodyRoot {
		t.Fatal("roundtrip mismatch")
	}
}

func TestBlockHeaderHashTreeRoot(t *testing.T) {
	h := &BlockHeader{
		Slot: 1, ProposerIndex: 0,
		ParentRoot: [32]byte{0x01}, StateRoot: [32]byte{0x02}, BodyRoot: [32]byte{0x03},
	}
	root, err := h.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root == [32]byte{} {
		t.Fatal("root should not be zero")
	}
}

func TestValidatorSSZRoundtrip(t *testing.T) {
	v := &Validator{AttestationPubkey: [52]byte{1, 2, 3}, ProposalPubkey: [52]byte{4, 5, 6}, Index: 7}
	data, err := v.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}
	v2 := &Validator{}
	if err := v2.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}
	if v.AttestationPubkey != v2.AttestationPubkey || v.ProposalPubkey != v2.ProposalPubkey || v.Index != v2.Index {
		t.Fatal("roundtrip mismatch")
	}
}

func TestValidatorDualKeysIndependent(t *testing.T) {
	// Verify attestation and proposal keys are stored independently.
	var attKey, propKey [52]byte
	for i := range attKey {
		attKey[i] = byte(i + 1)
	}
	for i := range propKey {
		propKey[i] = byte(i + 100)
	}
	v := &Validator{AttestationPubkey: attKey, ProposalPubkey: propKey, Index: 42}

	data, err := v.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 112 {
		t.Fatalf("expected 112 bytes, got %d", len(data))
	}

	v2 := &Validator{}
	if err := v2.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}
	if v2.AttestationPubkey != attKey {
		t.Fatal("attestation pubkey mismatch after roundtrip")
	}
	if v2.ProposalPubkey != propKey {
		t.Fatal("proposal pubkey mismatch after roundtrip")
	}
	if v2.AttestationPubkey == v2.ProposalPubkey {
		t.Fatal("keys should be different but are equal")
	}
	if v2.Index != 42 {
		t.Fatalf("index mismatch: got %d", v2.Index)
	}
}

func TestMaxAttestationsDataConstant(t *testing.T) {
	if MaxAttestationsData != 8 {
		t.Fatalf("MaxAttestationsData: expected 8, got %d", MaxAttestationsData)
	}
}

func TestAttestationDataSSZRoundtrip(t *testing.T) {
	d := &AttestationData{
		Slot:   5,
		Head:   &Checkpoint{Root: [32]byte{1}, Slot: 5},
		Target: &Checkpoint{Root: [32]byte{2}, Slot: 4},
		Source: &Checkpoint{Root: [32]byte{3}, Slot: 3},
	}
	data, err := d.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}
	d2 := &AttestationData{}
	if err := d2.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}
	if d.Slot != d2.Slot || d.Head.Slot != d2.Head.Slot || d.Target.Root != d2.Target.Root {
		t.Fatal("roundtrip mismatch")
	}
}

func TestAttestationDataHashTreeRoot(t *testing.T) {
	d := &AttestationData{
		Slot:   5,
		Head:   &Checkpoint{Root: [32]byte{1}, Slot: 5},
		Target: &Checkpoint{Root: [32]byte{2}, Slot: 4},
		Source: &Checkpoint{Root: [32]byte{3}, Slot: 3},
	}
	root, err := d.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root == [32]byte{} {
		t.Fatal("root should not be zero")
	}
	// Different data should produce different root
	d.Slot = 99
	root2, _ := d.HashTreeRoot()
	if root == root2 {
		t.Fatal("different data should produce different roots")
	}
}

func TestChainConfigSSZRoundtrip(t *testing.T) {
	c := &ChainConfig{GenesisTime: 1704085200}
	data, err := c.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}
	c2 := &ChainConfig{}
	if err := c2.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}
	if c.GenesisTime != c2.GenesisTime {
		t.Fatal("roundtrip mismatch")
	}
}

func TestStateSSZRoundtrip(t *testing.T) {
	s := &State{
		Config:            &ChainConfig{GenesisTime: 1000},
		Slot:              10,
		LatestBlockHeader: &BlockHeader{Slot: 9},
		LatestJustified:   &Checkpoint{Slot: 5},
		LatestFinalized:   &Checkpoint{Slot: 3},
		HistoricalBlockHashes: [][]byte{
			make([]byte, 32),
		},
		JustifiedSlots:           NewBitlistSSZ(10),
		Validators:               []*Validator{{AttestationPubkey: [52]byte{1}, Index: 0}},
		JustificationsRoots:      [][]byte{make([]byte, 32)},
		JustificationsValidators: NewBitlistSSZ(5),
	}
	data, err := s.MarshalSSZ()
	if err != nil {
		t.Fatal(err)
	}
	s2 := &State{}
	if err := s2.UnmarshalSSZ(data); err != nil {
		t.Fatal(err)
	}
	if s.Slot != s2.Slot || s.Config.GenesisTime != s2.Config.GenesisTime {
		t.Fatal("roundtrip mismatch")
	}
	if len(s.Validators) != len(s2.Validators) {
		t.Fatal("validator count mismatch")
	}
}

func TestStateHashTreeRoot(t *testing.T) {
	s := &State{
		Config:                   &ChainConfig{GenesisTime: 1000},
		Slot:                     10,
		LatestBlockHeader:        &BlockHeader{Slot: 9},
		LatestJustified:          &Checkpoint{Slot: 5},
		LatestFinalized:          &Checkpoint{Slot: 3},
		HistoricalBlockHashes:    [][]byte{make([]byte, 32)},
		JustifiedSlots:           NewBitlistSSZ(10),
		Validators:               []*Validator{{AttestationPubkey: [52]byte{1}, Index: 0}},
		JustificationsRoots:      [][]byte{make([]byte, 32)},
		JustificationsValidators: NewBitlistSSZ(5),
	}
	root, err := s.HashTreeRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root == [32]byte{} {
		t.Fatal("state root should not be zero")
	}
	// Deterministic
	root2, _ := s.HashTreeRoot()
	if root != root2 {
		t.Fatal("hash_tree_root not deterministic")
	}
}
