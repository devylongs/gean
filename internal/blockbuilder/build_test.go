package blockbuilder

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/attestationproof"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/types"
)

func minimalHeadState(t *testing.T) (*types.State, [32]byte) {
	t.Helper()

	state := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     0,
		LatestBlockHeader:        &types.BlockHeader{Slot: 0},
		LatestJustified:          &types.Checkpoint{Slot: 0},
		LatestFinalized:          &types.Checkpoint{Slot: 0},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
		Validators:               []*types.Validator{{}},
	}

	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute state root: %v", err)
	}
	state.LatestBlockHeader.StateRoot = stateRoot

	parentRoot, err := state.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute parent root: %v", err)
	}
	return state, parentRoot
}

func postHeaderVoteInput(t *testing.T) (*types.State, [32]byte, *types.AttestationData, [32]byte) {
	t.Helper()

	root0 := [32]byte{0x10}
	root1 := [32]byte{0x11}
	headState := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     2,
		LatestBlockHeader:        &types.BlockHeader{Slot: 2},
		LatestJustified:          &types.Checkpoint{Slot: 1, Root: root1},
		LatestFinalized:          &types.Checkpoint{Slot: 0, Root: root0},
		JustifiedSlots:           types.NewBitlistSSZ(2),
		JustificationsValidators: types.NewBitlistSSZ(0),
		Validators:               []*types.Validator{{}},
		HistoricalBlockHashes:    [][]byte{copyRoot(root0), copyRoot(root1)},
	}
	types.BitlistSet(headState.JustifiedSlots, 0)

	stateRoot, err := headState.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute state root: %v", err)
	}
	headState.LatestBlockHeader.StateRoot = stateRoot
	parentRoot, err := headState.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute parent root: %v", err)
	}

	data := &types.AttestationData{
		Slot:   2,
		Head:   &types.Checkpoint{Slot: 2, Root: parentRoot},
		Source: &types.Checkpoint{Slot: 1, Root: root1},
		Target: &types.Checkpoint{Slot: 2, Root: parentRoot},
	}
	dataRoot, err := data.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash attestation data: %v", err)
	}
	return headState, parentRoot, data, dataRoot
}

func copyRoot(root [32]byte) []byte {
	out := make([]byte, len(root))
	copy(out, root[:])
	return out
}

func TestBuildBlockSucceedsWithNoPayloads(t *testing.T) {
	headState, parentRoot := minimalHeadState(t)

	result, err := Build(Input{
		HeadState:     headState,
		Slot:          1,
		ProposerIndex: 0,
		ParentRoot:    parentRoot,
	})

	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if result == nil || result.Block == nil || result.Block.Body == nil {
		t.Fatal("expected block with body")
	}
	if len(result.AttestationProofs) != 0 {
		t.Fatalf("signatures=%d, want 0", len(result.AttestationProofs))
	}
	if len(result.Block.Body.Attestations) != 0 {
		t.Fatalf("attestations=%d, want 0", len(result.Block.Body.Attestations))
	}
}

func TestBuildBlockRejectsMalformedHeadState(t *testing.T) {
	_, err := Build(Input{
		HeadState:     &types.State{},
		Slot:          1,
		ProposerIndex: 0,
	})
	if err == nil {
		t.Fatal("expected malformed head state error")
	}
	if !errors.Is(err, ErrMalformedInput) {
		t.Fatalf("error=%v, want ErrMalformedInput", err)
	}
}

func TestBuildBlockRejectsPayloadsWithoutKnownRoots(t *testing.T) {
	headState, parentRoot, data, dataRoot := postHeaderVoteInput(t)

	result, err := Build(Input{
		HeadState:     headState,
		Slot:          3,
		ProposerIndex: 0,
		ParentRoot:    parentRoot,
		Payloads: []AttestationPayload{{
			DataRoot: dataRoot,
			Data:     data,
			Proofs:   []*types.SingleMessageAggregate{mockProof([]uint64{0})},
		}},
	})

	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
	if !errors.Is(err, ErrMalformedInput) {
		t.Fatalf("expected ErrMalformedInput, got %v", err)
	}
}

func TestBuildBlockReturnsTransitionError(t *testing.T) {
	headState, parentRoot := minimalHeadState(t)
	headState.Validators = []*types.Validator{{}, {}}

	result, err := Build(Input{
		HeadState:     headState,
		Slot:          1,
		ProposerIndex: 0,
		ParentRoot:    parentRoot,
	})

	if result != nil {
		t.Fatalf("expected nil result, got %v", result)
	}
	var proposerErr *statetransition.InvalidProposerError
	if !errors.As(err, &proposerErr) {
		t.Fatalf("error=%v, want InvalidProposerError", err)
	}
}

func TestBuildBlockReportsMismatchedPayloadRoot(t *testing.T) {
	headState, parentRoot := minimalHeadState(t)
	data := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Slot: 0, Root: parentRoot},
		Source: &types.Checkpoint{Slot: 0, Root: parentRoot},
		Target: &types.Checkpoint{Slot: 0, Root: parentRoot},
	}

	result, err := Build(Input{
		HeadState:       headState,
		Slot:            1,
		ProposerIndex:   0,
		ParentRoot:      parentRoot,
		KnownBlockRoots: RootSet{parentRoot: true},
		Payloads: []AttestationPayload{{
			DataRoot: [32]byte{0xee},
			Data:     data,
			Proofs:   []*types.SingleMessageAggregate{mockProof([]uint64{0})},
		}},
	})
	if err != nil {
		t.Fatalf("build block: %v", err)
	}
	if result == nil || result.Block == nil || result.Block.Body == nil {
		t.Fatal("expected block result")
	}
	if len(result.Block.Body.Attestations) != 0 {
		t.Fatalf("attestations=%d, want 0", len(result.Block.Body.Attestations))
	}
	if len(result.PayloadErrors) != 1 {
		t.Fatalf("payload errors=%d, want 1", len(result.PayloadErrors))
	}
	if !errors.Is(result.PayloadErrors[0].Err, ErrPayloadRootMismatch) {
		t.Fatalf("payload error=%v, want ErrPayloadRootMismatch", result.PayloadErrors[0].Err)
	}
}

func TestBuildBlockReportsSkippedPayloadIssues(t *testing.T) {
	headState, parentRoot, data, dataRoot := postHeaderVoteInput(t)
	// Accepting data advances justification to slot 2, so this vote's slot-1
	// source is no longer the current justified checkpoint and is skipped.
	staleSourceVote := *data
	staleSourceVote.Target = &types.Checkpoint{Slot: data.Source.Slot, Root: data.Source.Root}
	staleRoot := hashAttestationData(t, &staleSourceVote)
	unknownHead := *data
	unknownHead.Head = &types.Checkpoint{Slot: data.Head.Slot, Root: [32]byte{0xbb}}
	unknownRoot := hashAttestationData(t, &unknownHead)

	result, err := Build(Input{
		HeadState:       headState,
		Slot:            3,
		ProposerIndex:   0,
		ParentRoot:      parentRoot,
		KnownBlockRoots: RootSet{parentRoot: true},
		Payloads: []AttestationPayload{
			{DataRoot: dataRoot, Data: data, Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{0})}},
			{DataRoot: staleRoot, Data: &staleSourceVote, Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{0})}},
			{DataRoot: unknownRoot, Data: &unknownHead, Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{0})}},
		},
	})
	if err != nil {
		t.Fatalf("build block: %v", err)
	}
	if result == nil || result.Block == nil || result.Block.Body == nil {
		t.Fatal("expected block result")
	}
	if len(result.Block.Body.Attestations) != 1 {
		t.Fatalf("attestations=%d, want 1", len(result.Block.Body.Attestations))
	}
	if len(result.PayloadErrors) != 2 {
		t.Fatalf("payload errors=%d, want 2", len(result.PayloadErrors))
	}
	var foundStaleSource, foundUnknownHead bool
	for _, payloadErr := range result.PayloadErrors {
		foundStaleSource = foundStaleSource || errors.Is(payloadErr.Err, ErrPayloadSourceNotCurrentJustified)
		foundUnknownHead = foundUnknownHead || errors.Is(payloadErr.Err, ErrPayloadHeadUnknown)
	}
	if !foundStaleSource || !foundUnknownHead {
		t.Fatalf("payload errors=%v, want stale source and unknown head", result.PayloadErrors)
	}
}

func TestBuildBlockRecordsProofMergeFallback(t *testing.T) {
	headState, parentRoot, data, dataRoot := postHeaderVoteInput(t)

	result, err := Build(Input{
		HeadState:       headState,
		Slot:            3,
		ProposerIndex:   0,
		ParentRoot:      parentRoot,
		KnownBlockRoots: RootSet{parentRoot: true},
		Payloads: []AttestationPayload{{
			DataRoot: dataRoot,
			Data:     data,
			Proofs: []*types.SingleMessageAggregate{
				mockProof([]uint64{0}),
				mockProof([]uint64{1}),
			},
		}},
	})
	if err != nil {
		t.Fatalf("build block: %v", err)
	}
	if result == nil || result.Block == nil || result.Block.Body == nil {
		t.Fatal("expected block result")
	}
	if len(result.Block.Body.Attestations) != 1 {
		t.Fatalf("attestations=%d, want 1", len(result.Block.Body.Attestations))
	}
	if len(result.PayloadErrors) != 1 {
		t.Fatalf("payload errors=%d, want 1", len(result.PayloadErrors))
	}
	if !errors.Is(result.PayloadErrors[0].Err, attestationproof.ErrMergeUnavailable) {
		t.Fatalf("payload error=%v, want ErrMergeUnavailable", result.PayloadErrors[0].Err)
	}
}

func TestPlanAttestationsUsesPostHeaderState(t *testing.T) {
	headState, parentRoot, data, dataRoot := postHeaderVoteInput(t)

	plan, err := planAttestations(Input{
		HeadState:       headState,
		Slot:            3,
		ProposerIndex:   0,
		ParentRoot:      parentRoot,
		KnownBlockRoots: RootSet{parentRoot: true},
		Payloads: []AttestationPayload{{
			DataRoot: dataRoot,
			Data:     data,
			Proofs:   []*types.SingleMessageAggregate{mockProof([]uint64{0})},
		}},
	})
	if err != nil {
		t.Fatalf("plan attestations: %v", err)
	}
	if len(plan.payloadErrors) != 0 {
		t.Fatalf("payload errors=%d, want 0", len(plan.payloadErrors))
	}
	if len(plan.attestations) != 1 || len(plan.proofs) != 1 {
		t.Fatalf("planned attestations=%d proofs=%d, want 1/1", len(plan.attestations), len(plan.proofs))
	}
}

func TestPlanAttestationsCarriesTrialState(t *testing.T) {
	root0 := [32]byte{0x10}
	root1 := [32]byte{0x11}
	headState := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     2,
		LatestBlockHeader:        &types.BlockHeader{Slot: 2},
		LatestJustified:          &types.Checkpoint{Slot: 0, Root: root0},
		LatestFinalized:          &types.Checkpoint{Slot: 0, Root: root0},
		JustifiedSlots:           types.NewBitlistSSZ(2),
		JustificationsValidators: types.NewBitlistSSZ(0),
		Validators:               []*types.Validator{{}},
		HistoricalBlockHashes:    [][]byte{root0[:], root1[:]},
	}
	stateRoot, err := headState.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute state root: %v", err)
	}
	headState.LatestBlockHeader.StateRoot = stateRoot
	parentRoot, err := headState.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute parent root: %v", err)
	}

	first := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Slot: 1, Root: root1},
		Source: &types.Checkpoint{Slot: 0, Root: root0},
		Target: &types.Checkpoint{Slot: 1, Root: root1},
	}
	second := &types.AttestationData{
		Slot:   2,
		Head:   &types.Checkpoint{Slot: 2, Root: parentRoot},
		Source: &types.Checkpoint{Slot: 1, Root: root1},
		Target: &types.Checkpoint{Slot: 2, Root: parentRoot},
	}
	firstRoot, err := first.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash first attestation: %v", err)
	}
	secondRoot, err := second.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash second attestation: %v", err)
	}

	plan, err := planAttestations(Input{
		HeadState:       headState,
		Slot:            3,
		ProposerIndex:   0,
		ParentRoot:      parentRoot,
		KnownBlockRoots: RootSet{root1: true, parentRoot: true},
		Payloads: []AttestationPayload{
			{DataRoot: firstRoot, Data: first, Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{0})}},
			{DataRoot: secondRoot, Data: second, Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{0})}},
		},
	})
	if err != nil {
		t.Fatalf("plan attestations: %v", err)
	}
	if len(plan.payloadErrors) != 0 {
		t.Fatalf("payload errors=%d, want 0", len(plan.payloadErrors))
	}
	if len(plan.attestations) != 2 || len(plan.proofs) != 2 {
		t.Fatalf("planned attestations=%d proofs=%d, want 2/2", len(plan.attestations), len(plan.proofs))
	}
	if plan.postState.LatestJustified.Slot != 2 {
		t.Fatalf("latest justified slot=%d, want 2", plan.postState.LatestJustified.Slot)
	}
}

func TestPlanAttestationsContinuesWhenJustifiedSlotsChange(t *testing.T) {
	var roots [6][32]byte
	history := make([][]byte, len(roots))
	for i := range roots {
		roots[i] = [32]byte{byte(0x20 + i)}
		history[i] = copyRoot(roots[i])
	}
	headState := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     6,
		LatestBlockHeader:        &types.BlockHeader{Slot: 6},
		LatestJustified:          &types.Checkpoint{Slot: 0, Root: roots[0]},
		LatestFinalized:          &types.Checkpoint{Slot: 0, Root: roots[0]},
		JustifiedSlots:           types.NewBitlistSSZ(6),
		JustificationsValidators: types.NewBitlistSSZ(0),
		Validators:               []*types.Validator{{}},
		HistoricalBlockHashes:    history,
	}
	stateRoot, err := headState.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute state root: %v", err)
	}
	headState.LatestBlockHeader.StateRoot = stateRoot
	parentRoot, err := headState.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute parent root: %v", err)
	}

	first := &types.AttestationData{
		Slot:   2,
		Head:   &types.Checkpoint{Slot: 2, Root: roots[2]},
		Source: &types.Checkpoint{Slot: 0, Root: roots[0]},
		Target: &types.Checkpoint{Slot: 2, Root: roots[2]},
	}
	second := &types.AttestationData{
		Slot:   6,
		Head:   &types.Checkpoint{Slot: 6, Root: parentRoot},
		Source: &types.Checkpoint{Slot: 2, Root: roots[2]},
		Target: &types.Checkpoint{Slot: 6, Root: parentRoot},
	}
	firstRoot := hashAttestationData(t, first)
	secondRoot := hashAttestationData(t, second)

	plan, err := planAttestations(Input{
		HeadState:       headState,
		Slot:            7,
		ProposerIndex:   0,
		ParentRoot:      parentRoot,
		KnownBlockRoots: RootSet{roots[2]: true, parentRoot: true},
		Payloads: []AttestationPayload{
			{DataRoot: firstRoot, Data: first, Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{0})}},
			{DataRoot: secondRoot, Data: second, Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{0})}},
		},
	})
	if err != nil {
		t.Fatalf("plan attestations: %v", err)
	}
	if len(plan.payloadErrors) != 0 {
		t.Fatalf("payload errors=%d, want 0", len(plan.payloadErrors))
	}
	if len(plan.attestations) != 2 || len(plan.proofs) != 2 {
		t.Fatalf("planned attestations=%d proofs=%d, want 2/2", len(plan.attestations), len(plan.proofs))
	}
	if plan.postState.LatestJustified.Slot != 6 {
		t.Fatalf("latest justified slot=%d, want 6", plan.postState.LatestJustified.Slot)
	}
}

func TestPlanAttestationsDoesNotReportSkippedPayloadsWhenFull(t *testing.T) {
	root0 := [32]byte{0x30}
	headState := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     1,
		LatestBlockHeader:        &types.BlockHeader{Slot: 1},
		LatestJustified:          &types.Checkpoint{Slot: 0, Root: root0},
		LatestFinalized:          &types.Checkpoint{Slot: 0, Root: root0},
		JustifiedSlots:           types.NewBitlistSSZ(1),
		JustificationsValidators: types.NewBitlistSSZ(0),
		Validators:               []*types.Validator{{}},
		HistoricalBlockHashes:    [][]byte{copyRoot(root0)},
	}
	stateRoot, err := headState.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute state root: %v", err)
	}
	headState.LatestBlockHeader.StateRoot = stateRoot
	parentRoot, err := headState.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		t.Fatalf("compute parent root: %v", err)
	}

	source := &types.Checkpoint{Slot: 0, Root: root0}
	target := &types.Checkpoint{Slot: 1, Root: parentRoot}
	payloads := make([]AttestationPayload, 0, int(types.MaxAttestationsData)+1)
	for i := range int(types.MaxAttestationsData) {
		data := &types.AttestationData{
			Slot:   uint64(i),
			Head:   &types.Checkpoint{Slot: 1, Root: parentRoot},
			Source: source,
			Target: target,
		}
		payloads = append(payloads, AttestationPayload{
			DataRoot: hashAttestationData(t, data),
			Data:     data,
			Proofs:   []*types.SingleMessageAggregate{mockProof([]uint64{0})},
		})
	}
	unknownHead := &types.AttestationData{
		Slot:   uint64(types.MaxAttestationsData),
		Head:   &types.Checkpoint{Slot: 1, Root: [32]byte{0xff}},
		Source: source,
		Target: target,
	}
	payloads = append(payloads, AttestationPayload{
		DataRoot: hashAttestationData(t, unknownHead),
		Data:     unknownHead,
		Proofs:   []*types.SingleMessageAggregate{mockProof([]uint64{0})},
	})

	plan, err := planAttestations(Input{
		HeadState:       headState,
		Slot:            2,
		ProposerIndex:   0,
		ParentRoot:      parentRoot,
		KnownBlockRoots: RootSet{parentRoot: true},
		Payloads:        payloads,
	})
	if err != nil {
		t.Fatalf("plan attestations: %v", err)
	}
	if len(plan.attestations) != int(types.MaxAttestationsData) {
		t.Fatalf("planned attestations=%d, want %d", len(plan.attestations), types.MaxAttestationsData)
	}
	if len(plan.payloadErrors) != 0 {
		t.Fatalf("payload errors=%d, want 0", len(plan.payloadErrors))
	}
}
