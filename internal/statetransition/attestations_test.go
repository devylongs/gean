package statetransition

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestLatestJustifiedDoesNotRegressWithinBlock(t *testing.T) {
	const numValidators = 4

	var r3, r4, r6, r9 [types.RootSize]byte
	r3[0] = 3
	r4[0] = 4
	r6[0] = 6
	r9[0] = 9

	hashes := make([][]byte, 10)
	for i := range hashes {
		hashes[i] = make([]byte, types.RootSize)
	}
	copy(hashes[3], r3[:])
	copy(hashes[4], r4[:])
	copy(hashes[6], r6[:])
	copy(hashes[9], r9[:])

	state := makeGenesisState(numValidators)
	state.LatestJustified = &types.Checkpoint{Slot: 3, Root: r3}
	state.LatestFinalized = &types.Checkpoint{}
	state.HistoricalBlockHashes = hashes
	// Header processing would size the window to cover every voted slot; do the
	// same here so the slot-9 vote is in range rather than a spurious rejection.
	state.JustifiedSlots = types.NewBitlistSSZ(9)
	setSlotJustified(state, 0, 3)

	bits := []byte{0x17}

	mkAtt := func(targetSlot uint64, targetRoot [types.RootSize]byte) *types.AggregatedAttestation {
		return &types.AggregatedAttestation{
			AggregationBits: bits,
			Data: &types.AttestationData{
				Slot:   targetSlot,
				Head:   &types.Checkpoint{Slot: targetSlot, Root: targetRoot},
				Target: &types.Checkpoint{Slot: targetSlot, Root: targetRoot},
				Source: &types.Checkpoint{Slot: 3, Root: r3},
			},
		}
	}

	atts := []*types.AggregatedAttestation{
		mkAtt(4, r4),
		mkAtt(9, r9),
		mkAtt(6, r6),
	}

	if err := ProcessAttestations(state, atts); err != nil {
		t.Fatalf("ProcessAttestations: %v", err)
	}

	if state.LatestJustified.Slot != 9 {
		t.Fatalf("LatestJustified.Slot = %d, want 9 (monotonic guard violated — slot 6 dragged checkpoint back)",
			state.LatestJustified.Slot)
	}
	if state.LatestJustified.Root != r9 {
		t.Fatalf("LatestJustified.Root = %x, want %x", state.LatestJustified.Root, r9)
	}
}

func TestProcessAttestationsStaleSourceJustifiesWithoutReFinalizing(t *testing.T) {
	const numValidators = 4

	var r1, r4, r6 [types.RootSize]byte
	r1[0] = 1
	r4[0] = 4
	r6[0] = 6

	hashes := make([][]byte, 7)
	for i := range hashes {
		hashes[i] = make([]byte, types.RootSize)
	}
	copy(hashes[1], r1[:])
	copy(hashes[4], r4[:])
	copy(hashes[6], r6[:])

	state := makeGenesisState(numValidators)
	state.LatestJustified = &types.Checkpoint{Slot: 4, Root: r4}
	state.LatestFinalized = &types.Checkpoint{Slot: 4, Root: r4}
	state.HistoricalBlockHashes = hashes
	// Window covers slots 5..6 above the finalized boundary; the slot-1 source is
	// below it and justified by definition, so only the slot-6 target is tracked.
	state.JustifiedSlots = types.NewBitlistSSZ(2)

	att := &types.AggregatedAttestation{
		AggregationBits: types.BitlistFromIndices([]uint64{0, 1, 2}),
		Data: &types.AttestationData{
			Slot:   7,
			Head:   &types.Checkpoint{Slot: 6, Root: r6},
			Target: &types.Checkpoint{Slot: 6, Root: r6},
			Source: &types.Checkpoint{Slot: 1, Root: r1},
		},
	}

	if err := ProcessAttestations(state, []*types.AggregatedAttestation{att}); err != nil {
		t.Fatalf("ProcessAttestations: %v", err)
	}
	if state.LatestJustified.Slot != 6 || state.LatestJustified.Root != r6 {
		t.Fatalf("latest justified=%+v, want slot 6 root %x", state.LatestJustified, r6)
	}
	if state.LatestFinalized.Slot != 4 || state.LatestFinalized.Root != r4 {
		t.Fatalf("latest finalized=%+v, want slot 4 root %x", state.LatestFinalized, r4)
	}
	if !types.BitlistGet(state.JustifiedSlots, 1) {
		t.Fatalf("target slot 6 was not recorded in justified slots: %08b", state.JustifiedSlots)
	}
	if len(state.JustificationsRoots) != 0 || types.BitlistLen(state.JustificationsValidators) != 0 {
		t.Fatalf("pending justifications roots=%d validators_len=%d, want none",
			len(state.JustificationsRoots), types.BitlistLen(state.JustificationsValidators))
	}
}

func TestProcessAttestationsRejectsMalformedState(t *testing.T) {
	if err := ProcessAttestations(nil, nil); !errors.Is(err, ErrMalformedState) {
		t.Fatalf("nil state error=%v, want ErrMalformedState", err)
	}

	state := makeGenesisState(1)
	state.LatestJustified = nil
	if err := ProcessAttestations(state, nil); !errors.Is(err, ErrMalformedState) {
		t.Fatalf("nil latest justified error=%v, want ErrMalformedState", err)
	}

	state = makeGenesisState(1)
	state.LatestFinalized = nil
	if err := ProcessAttestations(state, nil); !errors.Is(err, ErrMalformedState) {
		t.Fatalf("nil latest finalized error=%v, want ErrMalformedState", err)
	}
}

func TestProcessAttestationsSkipsMalformedAttestations(t *testing.T) {
	state := makeGenesisState(2)
	atts := []*types.AggregatedAttestation{
		nil,
		{},
		{Data: &types.AttestationData{}},
		{Data: &types.AttestationData{Source: &types.Checkpoint{}}},
	}

	if err := ProcessAttestations(state, atts); err != nil {
		t.Fatalf("ProcessAttestations malformed attestations: %v", err)
	}
	if state.LatestJustified.Slot != 0 {
		t.Fatalf("latest justified slot=%d, want 0", state.LatestJustified.Slot)
	}
}

func TestProcessAttestationsCopiesCheckpointInputs(t *testing.T) {
	state := makeGenesisState(3)
	sourceRoot := [32]byte{0x01}
	targetRoot := [32]byte{0x02}
	state.LatestFinalized = &types.Checkpoint{Slot: 0, Root: sourceRoot}
	state.HistoricalBlockHashes = [][]byte{
		append([]byte(nil), sourceRoot[:]...),
		make([]byte, types.RootSize),
		append([]byte(nil), targetRoot[:]...),
	}
	state.JustifiedSlots = types.NewBitlistSSZ(2)

	source := &types.Checkpoint{Slot: 0, Root: sourceRoot}
	target := &types.Checkpoint{Slot: 2, Root: targetRoot}
	att := &types.AggregatedAttestation{
		AggregationBits: types.BitlistFromIndices([]uint64{0, 1}),
		Data: &types.AttestationData{
			Slot:   2,
			Head:   target,
			Source: source,
			Target: target,
		},
	}

	if err := ProcessAttestations(state, []*types.AggregatedAttestation{att}); err != nil {
		t.Fatalf("ProcessAttestations: %v", err)
	}

	source.Root = [32]byte{0xaa}
	source.Slot = 99
	target.Root = [32]byte{0xbb}
	target.Slot = 100

	if state.LatestJustified.Slot != 2 || state.LatestJustified.Root != targetRoot {
		t.Fatalf("latest justified=%+v, want slot 2 root %x", state.LatestJustified, targetRoot)
	}
	if state.LatestFinalized.Slot != 0 || state.LatestFinalized.Root != sourceRoot {
		t.Fatalf("latest finalized=%+v, want slot 0 root %x", state.LatestFinalized, sourceRoot)
	}
}

func TestSerializeJustificationsSortsAndCopiesRoots(t *testing.T) {
	state := makeGenesisState(2)
	rootA := [32]byte{0x01}
	rootB := [32]byte{0x02}
	serializeJustifications(state, map[[32]byte][]bool{
		rootB: {false, true},
		rootA: {true, false},
	}, 2)

	if len(state.JustificationsRoots) != 2 {
		t.Fatalf("justification roots=%d, want 2", len(state.JustificationsRoots))
	}
	if state.JustificationsRoots[0][0] != rootA[0] || state.JustificationsRoots[1][0] != rootB[0] {
		t.Fatalf("roots not sorted: %x %x", state.JustificationsRoots[0], state.JustificationsRoots[1])
	}

	state.JustificationsRoots[0][0] = 0xff
	again := sortedJustificationRoots(map[[32]byte][]bool{rootA: nil})
	if again[0] != rootA {
		t.Fatal("serialized root alias mutated source root")
	}
}

func TestProcessAttestationsRequiresHeadOnChain(t *testing.T) {
	const numValidators = 4
	var r0, r2 [types.RootSize]byte
	r0[0] = 0xa0
	r2[0] = 0xa2

	mkState := func() *types.State {
		hashes := make([][]byte, 3)
		for i := range hashes {
			hashes[i] = make([]byte, types.RootSize)
		}
		copy(hashes[0], r0[:])
		copy(hashes[2], r2[:])
		s := makeGenesisState(numValidators)
		s.LatestJustified = &types.Checkpoint{Slot: 0, Root: r0}
		s.LatestFinalized = &types.Checkpoint{Slot: 0, Root: r0}
		s.HistoricalBlockHashes = hashes
		s.JustifiedSlots = types.NewBitlistSSZ(2)
		return s
	}
	mkAtt := func(head *types.Checkpoint) *types.AggregatedAttestation {
		return &types.AggregatedAttestation{
			AggregationBits: types.BitlistFromIndices([]uint64{0, 1, 2}),
			Data: &types.AttestationData{
				Slot:   2,
				Head:   head,
				Source: &types.Checkpoint{Slot: 0, Root: r0},
				Target: &types.Checkpoint{Slot: 2, Root: r2},
			},
		}
	}

	t.Run("on_chain_head_justifies", func(t *testing.T) {
		s := mkState()
		if err := ProcessAttestations(s, []*types.AggregatedAttestation{mkAtt(&types.Checkpoint{Slot: 2, Root: r2})}); err != nil {
			t.Fatalf("ProcessAttestations: %v", err)
		}
		if s.LatestJustified.Slot != 2 {
			t.Fatalf("LatestJustified.Slot = %d, want 2", s.LatestJustified.Slot)
		}
	})

	t.Run("off_chain_head_skipped", func(t *testing.T) {
		var bogus [types.RootSize]byte
		bogus[0] = 0xee
		s := mkState()
		if err := ProcessAttestations(s, []*types.AggregatedAttestation{mkAtt(&types.Checkpoint{Slot: 2, Root: bogus})}); err != nil {
			t.Fatalf("ProcessAttestations: %v", err)
		}
		if s.LatestJustified.Slot != 0 {
			t.Fatalf("LatestJustified.Slot = %d, want 0 (off-chain head must be skipped)", s.LatestJustified.Slot)
		}
	})
}

func bitsBoundsHarness() (*types.State, func(bits []byte) *types.AggregatedAttestation) {
	var r3, r4 [types.RootSize]byte
	r3[0] = 3
	r4[0] = 4
	hashes := make([][]byte, 10)
	for i := range hashes {
		hashes[i] = make([]byte, types.RootSize)
	}
	copy(hashes[3], r3[:])
	copy(hashes[4], r4[:])

	state := makeGenesisState(4)
	state.LatestJustified = &types.Checkpoint{Slot: 3, Root: r3}
	state.LatestFinalized = &types.Checkpoint{}
	state.HistoricalBlockHashes = hashes
	// Window covers slots 1..4 so the slot-4 target is in range; slot-3 source is
	// justified within it.
	state.JustifiedSlots = types.NewBitlistSSZ(4)
	setSlotJustified(state, 0, 3)

	mkAtt := func(bits []byte) *types.AggregatedAttestation {
		return &types.AggregatedAttestation{
			AggregationBits: bits,
			Data: &types.AttestationData{
				Slot:   4,
				Head:   &types.Checkpoint{Slot: 4, Root: r4},
				Target: &types.Checkpoint{Slot: 4, Root: r4},
				Source: &types.Checkpoint{Slot: 3, Root: r3},
			},
		}
	}
	return state, mkAtt
}

func TestProcessAttestationsRejectsEmptyAggregationBits(t *testing.T) {
	state, mkAtt := bitsBoundsHarness()

	err := ProcessAttestations(state, []*types.AggregatedAttestation{mkAtt(types.NewBitlistSSZ(4))})
	if !errors.Is(err, ErrEmptyAggregationBits) {
		t.Fatalf("err = %v, want ErrEmptyAggregationBits", err)
	}
}

func TestProcessAttestationsRejectsOutOfRangeSetBit(t *testing.T) {
	state, mkAtt := bitsBoundsHarness()

	err := ProcessAttestations(state, []*types.AggregatedAttestation{mkAtt(types.BitlistFromIndices([]uint64{0, 7}))})
	var oor *AttesterIndexOutOfRangeError
	if !errors.As(err, &oor) {
		t.Fatalf("err = %v, want AttesterIndexOutOfRangeError", err)
	}
	if oor.Index != 7 || oor.Validators != 4 {
		t.Fatalf("index=%d validators=%d, want 7 and 4", oor.Index, oor.Validators)
	}
}

func TestProcessAttestationsToleratesUnsetPaddingBits(t *testing.T) {
	state, mkAtt := bitsBoundsHarness()

	// Bitlist longer than the registry, set bits all in range: a valid
	// supermajority whose votes must count despite the padding.
	bits := types.NewBitlistSSZ(8)
	for _, i := range []uint64{0, 1, 2} {
		types.BitlistSet(bits, i)
	}

	if err := ProcessAttestations(state, []*types.AggregatedAttestation{mkAtt(bits)}); err != nil {
		t.Fatalf("ProcessAttestations: %v", err)
	}
	if state.LatestJustified.Slot != 4 {
		t.Fatalf("LatestJustified.Slot = %d, want 4 (padded votes must be tallied)", state.LatestJustified.Slot)
	}
}
