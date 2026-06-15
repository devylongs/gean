package attestation_test

import (
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func makeValidationStore() *store.ConsensusStore {
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	s.SetConfig(&types.ChainConfig{GenesisTime: 1000})
	return s
}

func makeValidAttestationData() *types.AttestationData {
	return &types.AttestationData{
		Slot:   5,
		Source: &types.Checkpoint{Root: [32]byte{1}, Slot: 3},
		Target: &types.Checkpoint{Root: [32]byte{2}, Slot: 4},
		Head:   &types.Checkpoint{Root: [32]byte{3}, Slot: 5},
	}
}

func insertValidationHeaders(s *store.ConsensusStore) {
	s.InsertBlockHeader([32]byte{1}, &types.BlockHeader{Slot: 3})
	s.InsertBlockHeader([32]byte{2}, &types.BlockHeader{Slot: 4, ParentRoot: [32]byte{1}})
	s.InsertBlockHeader([32]byte{3}, &types.BlockHeader{Slot: 5, ParentRoot: [32]byte{2}})
}

func TestValidateAttestationDataAvailability(t *testing.T) {
	s := makeValidationStore()
	data := makeValidAttestationData()

	err := attestation.ValidateAttestationData(s, data)
	if err == nil {
		t.Fatal("should fail with unknown blocks")
	}
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrUnknownSourceBlock {
		t.Fatalf("expected UnknownSourceBlock, got %v", err)
	}
}

func TestValidateAttestationDataTopology(t *testing.T) {
	s := makeValidationStore()
	s.SetTime(30)
	insertValidationHeaders(s)

	data := makeValidAttestationData()
	if err := attestation.ValidateAttestationData(s, data); err != nil {
		t.Fatalf("should pass: %v", err)
	}

	bad := *data
	bad.Source = &types.Checkpoint{Root: [32]byte{3}, Slot: 5}
	bad.Target = &types.Checkpoint{Root: [32]byte{1}, Slot: 3}
	err := attestation.ValidateAttestationData(s, &bad)
	if err == nil {
		t.Fatal("should fail: source exceeds target")
	}
}

func TestValidateAttestationDataMalformedShape(t *testing.T) {
	s := makeValidationStore()
	tests := []struct {
		name string
		data *types.AttestationData
		want string
	}{
		{"nil_data", nil, "data"},
		{"nil_source", &types.AttestationData{Target: &types.Checkpoint{}, Head: &types.Checkpoint{}}, "source"},
		{"nil_target", &types.AttestationData{Source: &types.Checkpoint{}, Head: &types.Checkpoint{}}, "target"},
		{"nil_head", &types.AttestationData{Source: &types.Checkpoint{}, Target: &types.Checkpoint{}}, "head"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := attestation.ValidateAttestationData(s, tc.data)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%q, want mention %q", err.Error(), tc.want)
			}
		})
	}
}

func TestValidateAttestationDataSlotMismatches(t *testing.T) {
	tests := []struct {
		name string
		edit func(*types.AttestationData)
		kind store.StoreErrorKind
	}{
		{"source", func(d *types.AttestationData) { d.Source.Slot = 2 }, store.ErrSourceSlotMismatch},
		{"target", func(d *types.AttestationData) { d.Target.Slot = 3 }, store.ErrTargetSlotMismatch},
		{"head", func(d *types.AttestationData) { d.Head.Slot = 4 }, store.ErrHeadSlotMismatch},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := makeValidationStore()
			s.SetTime(30)
			insertValidationHeaders(s)
			data := makeValidAttestationData()
			tc.edit(data)

			err := attestation.ValidateAttestationData(s, data)
			se, ok := err.(*store.StoreError)
			if !ok || se.Kind != tc.kind {
				t.Fatalf("error=%v, want StoreError kind %v", err, tc.kind)
			}
		})
	}
}

func TestValidateAttestationDataSlotBeforeHead(t *testing.T) {
	s := makeValidationStore()
	s.SetTime(30)
	insertValidationHeaders(s)

	data := makeValidAttestationData()
	// Vote slot precedes the head it claims to have seen (head slot 5).
	data.Slot = 4

	err := attestation.ValidateAttestationData(s, data)
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrAttestationSlotBeforeHead {
		t.Fatalf("error=%v, want ErrAttestationSlotBeforeHead", err)
	}
}

func TestValidateAttestationDataFutureSlot(t *testing.T) {
	s := makeValidationStore()
	s.SetTime(0)
	insertValidationHeaders(s)

	data := makeValidAttestationData()
	data.Slot = 100

	err := attestation.ValidateAttestationData(s, data)
	se, ok := err.(*store.StoreError)
	if !ok || se.Kind != store.ErrAttestationTooFarInFuture {
		t.Fatalf("error=%v, want ErrAttestationTooFarInFuture", err)
	}
}

func TestValidateAttestationDataAncestry(t *testing.T) {
	// Chain root1(3) <- root2(4) <- root3(5), plus a fork root4(4) off root1.
	newStore := func() *store.ConsensusStore {
		s := makeValidationStore()
		s.SetTime(30)
		insertValidationHeaders(s)
		s.InsertBlockHeader([32]byte{4}, &types.BlockHeader{Slot: 4, ParentRoot: [32]byte{1}})
		return s
	}
	cp := func(root byte, slot uint64) *types.Checkpoint {
		return &types.Checkpoint{Root: [32]byte{root}, Slot: slot}
	}

	tests := []struct {
		name           string
		source, target *types.Checkpoint
		head           *types.Checkpoint
		want           store.StoreErrorKind
	}{
		{
			name:   "source_not_ancestor_of_target",
			source: cp(4, 4), target: cp(3, 5), head: cp(3, 5),
			want: store.ErrSourceNotAncestorOfTarget,
		},
		{
			name:   "target_not_ancestor_of_head",
			source: cp(1, 3), target: cp(4, 4), head: cp(3, 5),
			want: store.ErrTargetNotAncestorOfHead,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore()
			data := &types.AttestationData{Slot: 5, Source: tc.source, Target: tc.target, Head: tc.head}
			err := attestation.ValidateAttestationData(s, data)
			se, ok := err.(*store.StoreError)
			if !ok || se.Kind != tc.want {
				t.Fatalf("error=%v, want kind %d", err, tc.want)
			}
		})
	}
}
