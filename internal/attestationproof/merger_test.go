package attestationproof

import (
	"errors"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func testData() *types.AttestationData {
	return &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{},
		Target: &types.Checkpoint{},
		Source: &types.Checkpoint{},
	}
}

func testProof(ids ...uint64) *types.SingleMessageAggregate {
	return &types.SingleMessageAggregate{
		Participants: types.BitlistFromIndices(ids),
		Proof:        []byte{0x01},
	}
}

func TestMergeRejectsNilCache(t *testing.T) {
	merger := NewMerger(nil)
	state := &types.State{Validators: []*types.Validator{{}, {}}}

	proof, err := merger.Merge(
		[]*types.SingleMessageAggregate{testProof(0), testProof(1)},
		testData(),
		state,
	)
	if proof != nil {
		t.Fatalf("proof=%v, want nil", proof)
	}
	if !errors.Is(err, ErrMergeUnavailable) {
		t.Fatalf("error=%v, want ErrMergeUnavailable", err)
	}
}

func TestMergeRejectsOutOfRangeParticipant(t *testing.T) {
	merger := NewMerger(xmss.NewPubKeyCache())
	defer merger.cache.Close()
	state := &types.State{Validators: []*types.Validator{{}}}

	proof, err := merger.Merge(
		[]*types.SingleMessageAggregate{testProof(0), testProof(1)},
		testData(),
		state,
	)
	if proof != nil {
		t.Fatalf("proof=%v, want nil", proof)
	}
	if !errors.Is(err, ErrMergeUnavailable) {
		t.Fatalf("error=%v, want ErrMergeUnavailable", err)
	}
}

func TestMergeRejectsMalformedProof(t *testing.T) {
	merger := NewMerger(xmss.NewPubKeyCache())
	defer merger.cache.Close()
	state := &types.State{Validators: []*types.Validator{{}, {}}}

	proof, err := merger.Merge(
		[]*types.SingleMessageAggregate{testProof(0), {Participants: types.BitlistFromIndices([]uint64{1})}},
		testData(),
		state,
	)
	if proof != nil {
		t.Fatalf("proof=%v, want nil", proof)
	}
	if !errors.Is(err, ErrMergeUnavailable) {
		t.Fatalf("error=%v, want ErrMergeUnavailable", err)
	}
}

func TestMergeRejectsOverlappingParticipants(t *testing.T) {
	merger := NewMerger(xmss.NewPubKeyCache())
	defer merger.cache.Close()
	state := &types.State{Validators: []*types.Validator{{}, {}, {}}}

	proof, err := merger.Merge(
		[]*types.SingleMessageAggregate{testProof(0, 1), testProof(1, 2)},
		testData(),
		state,
	)
	if proof != nil {
		t.Fatalf("proof=%v, want nil", proof)
	}
	if !errors.Is(err, ErrMergeUnavailable) || !strings.Contains(err.Error(), "appears in multiple proofs") {
		t.Fatalf("error=%v, want overlapping participant rejection", err)
	}
}

func TestMergeRejectsSlotOverflow(t *testing.T) {
	merger := NewMerger(nil)
	data := testData()
	data.Slot = uint64(^uint32(0)) + 1

	proof, err := merger.Merge(
		[]*types.SingleMessageAggregate{testProof(0), testProof(1)},
		data,
		&types.State{},
	)
	if proof != nil {
		t.Fatalf("proof=%v, want nil", proof)
	}
	if !errors.Is(err, ErrMergeUnavailable) || !strings.Contains(err.Error(), "overflows uint32") {
		t.Fatalf("error=%v, want slot overflow rejection", err)
	}
}
