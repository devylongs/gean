package blockbuilder

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestSortedPayloadsSkipsMalformedEntries(t *testing.T) {
	validData := mockAttestationData()
	validRoot, err := validData.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash valid data: %v", err)
	}
	payloads := []AttestationPayload{
		{DataRoot: [32]byte{0x01}},
		{DataRoot: [32]byte{0x02}, Data: nil},
		{DataRoot: [32]byte{0x03}, Data: &types.AttestationData{Head: &types.Checkpoint{}, Source: &types.Checkpoint{}}, Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{1})}},
		{DataRoot: validRoot, Data: validData, Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{1})}},
	}

	sorted, payloadErrors := sortedPayloads(payloads)
	if len(sorted) != 1 {
		t.Fatalf("sorted payloads=%d, want 1", len(sorted))
	}
	if len(payloadErrors) != 3 {
		t.Fatalf("payload errors=%d, want 3", len(payloadErrors))
	}
	for _, payloadErr := range payloadErrors {
		if !errors.Is(payloadErr.Err, ErrMalformedPayload) {
			t.Fatalf("payload error=%v, want ErrMalformedPayload", payloadErr.Err)
		}
	}
	if sorted[0].DataRoot != validRoot {
		t.Fatalf("data root=%x, want %x", sorted[0].DataRoot, validRoot)
	}
}

func TestValidatePayloadRejectsMalformedFields(t *testing.T) {
	tests := []struct {
		name    string
		payload AttestationPayload
	}{
		{
			name:    "nil data",
			payload: AttestationPayload{},
		},
		{
			name: "empty proofs",
			payload: AttestationPayload{
				Data: mockAttestationData(),
			},
		},
		{
			name: "nil head",
			payload: AttestationPayload{
				Data:   &types.AttestationData{Source: &types.Checkpoint{}, Target: &types.Checkpoint{}},
				Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{1})},
			},
		},
		{
			name: "nil target",
			payload: AttestationPayload{
				Data:   &types.AttestationData{Head: &types.Checkpoint{}, Source: &types.Checkpoint{}},
				Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{1})},
			},
		},
		{
			name: "nil source",
			payload: AttestationPayload{
				Data:   &types.AttestationData{Head: &types.Checkpoint{}, Target: &types.Checkpoint{}},
				Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{1})},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validatePayload(test.payload); !errors.Is(err, ErrMalformedPayload) {
				t.Fatalf("validatePayload() error=%v, want ErrMalformedPayload", err)
			}
		})
	}
}

func TestSortedPayloadsReportsMismatchedDataRoot(t *testing.T) {
	payloads := []AttestationPayload{{
		DataRoot: [32]byte{0x04},
		Data:     mockAttestationData(),
		Proofs:   []*types.SingleMessageAggregate{mockProof([]uint64{1})},
	}}

	sorted, payloadErrors := sortedPayloads(payloads)
	if len(sorted) != 0 {
		t.Fatalf("sorted payloads=%d, want 0", len(sorted))
	}
	if len(payloadErrors) != 1 {
		t.Fatalf("payload errors=%d, want 1", len(payloadErrors))
	}
	if !errors.Is(payloadErrors[0].Err, ErrPayloadRootMismatch) {
		t.Fatalf("payload error=%v, want ErrPayloadRootMismatch", payloadErrors[0].Err)
	}
}

func TestSortedPayloadsOrdersByTargetSlotThenRoot(t *testing.T) {
	lowSlot := mockAttestationData()
	lowSlot.Target.Slot = 4
	highLeft := mockAttestationData()
	highLeft.Target.Slot = 8
	highLeft.Head.Root = [32]byte{0x21}
	highRight := mockAttestationData()
	highRight.Target.Slot = 8
	highRight.Head.Root = [32]byte{0x22}

	lowRoot := hashAttestationData(t, lowSlot)
	leftRoot := hashAttestationData(t, highLeft)
	rightRoot := hashAttestationData(t, highRight)

	sorted, payloadErrors := sortedPayloads([]AttestationPayload{
		{DataRoot: rightRoot, Data: highRight, Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{1})}},
		{DataRoot: leftRoot, Data: highLeft, Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{1})}},
		{DataRoot: lowRoot, Data: lowSlot, Proofs: []*types.SingleMessageAggregate{mockProof([]uint64{1})}},
	})

	if len(payloadErrors) != 0 {
		t.Fatalf("payload errors=%d, want 0", len(payloadErrors))
	}
	if len(sorted) != 3 {
		t.Fatalf("sorted payloads=%d, want 3", len(sorted))
	}
	if sorted[0].DataRoot != lowRoot {
		t.Fatalf("first root=%x, want low slot root %x", sorted[0].DataRoot, lowRoot)
	}
	if compareRoots(sorted[1].DataRoot, sorted[2].DataRoot) > 0 {
		t.Fatalf("same-slot payloads sorted out of root order: %x after %x", sorted[1].DataRoot, sorted[2].DataRoot)
	}
}

func TestSortedPayloadsMergesDuplicateRoots(t *testing.T) {
	data := mockAttestationData()
	root := hashAttestationData(t, data)
	firstProof := mockProof([]uint64{1})
	secondProof := mockProof([]uint64{2})
	first := AttestationPayload{
		DataRoot: root,
		Data:     data,
		Proofs:   []*types.SingleMessageAggregate{firstProof},
	}

	sorted, payloadErrors := sortedPayloads([]AttestationPayload{
		first,
		{
			DataRoot: root,
			Data:     data,
			Proofs:   []*types.SingleMessageAggregate{secondProof},
		},
	})

	if len(payloadErrors) != 0 {
		t.Fatalf("payload errors=%d, want 0", len(payloadErrors))
	}
	if len(sorted) != 1 {
		t.Fatalf("sorted payloads=%d, want 1", len(sorted))
	}
	if len(sorted[0].Proofs) != 2 {
		t.Fatalf("proofs=%d, want 2", len(sorted[0].Proofs))
	}
	if sorted[0].Proofs[0] != firstProof || sorted[0].Proofs[1] != secondProof {
		t.Fatal("duplicate root proofs were not preserved in order")
	}

	sorted[0].Proofs[0] = nil
	if first.Proofs[0] == nil {
		t.Fatal("canonical payload proof slice aliases input slice")
	}
}

func TestPayloadBuildIssueUsesTransitionVoteRules(t *testing.T) {
	headState, parentRoot, data, dataRoot := postHeaderVoteInput(t)
	workingState, err := transitionBlock(headState, 3, newBlock(3, 0, parentRoot, nil))
	if err != nil {
		t.Fatalf("transition header: %v", err)
	}

	payload := AttestationPayload{
		DataRoot: dataRoot,
		Data:     data,
		Proofs:   []*types.SingleMessageAggregate{mockProof([]uint64{0})},
	}
	knownRoots := RootSet{parentRoot: true}
	if err := payloadBuildIssue(workingState, knownRoots, payload); err != nil {
		t.Fatalf("expected payload to be buildable, got %v", err)
	}

	if err := payloadBuildIssue(workingState, nil, payload); !errors.Is(err, ErrPayloadHeadUnknown) {
		t.Fatalf("payload build issue=%v, want ErrPayloadHeadUnknown", err)
	} else if !IsExpectedSkip(err) {
		t.Fatalf("head unknown skip was not marked expected: %v", err)
	}

	// Head known but off the canonical chain (a forked block) must be dropped,
	// matching the spec proposer; expected skip, not a hard error.
	offHead := *data
	forkHead := [32]byte{0xfe}
	offHead.Head = &types.Checkpoint{Slot: data.Head.Slot, Root: forkHead}
	if err := payloadBuildIssue(workingState, RootSet{parentRoot: true, forkHead: true},
		AttestationPayload{DataRoot: dataRoot, Data: &offHead, Proofs: payload.Proofs}); !errors.Is(err, ErrPayloadHeadOffChain) {
		t.Fatalf("payload build issue=%v, want ErrPayloadHeadOffChain", err)
	} else if !IsExpectedSkip(err) {
		t.Fatalf("off-chain head skip was not marked expected: %v", err)
	}

	sameSlot := *data
	sameSlot.Target = &types.Checkpoint{Slot: data.Source.Slot, Root: data.Source.Root}
	payload.Data = &sameSlot
	if err := payloadBuildIssue(workingState, knownRoots, payload); !errors.Is(err, ErrPayloadVoteInvalid) {
		t.Fatalf("payload build issue=%v, want ErrPayloadVoteInvalid", err)
	}

	// Chain mismatch must be tested before any slot is justified below, since
	// leanSpec checks target justification ahead of chain membership: an
	// already-justified target would otherwise mask the mismatch as an expected skip.
	wrongRoot := *data
	wrongRoot.Target = &types.Checkpoint{Slot: data.Target.Slot, Root: [32]byte{0x99}}
	payload.Data = &wrongRoot
	if err := payloadBuildIssue(workingState, knownRoots, payload); !errors.Is(err, ErrPayloadVoteInvalid) {
		t.Fatalf("payload build issue=%v, want ErrPayloadVoteInvalid", err)
	} else if IsExpectedSkip(err) {
		t.Fatalf("chain mismatch was marked expected: %v", err)
	}

	alreadyJustified := *data
	workingState.JustifiedSlots = types.BitlistExtend(workingState.JustifiedSlots, 2)
	types.BitlistSet(workingState.JustifiedSlots, 1)
	payload.Data = &alreadyJustified
	if err := payloadBuildIssue(workingState, knownRoots, payload); !errors.Is(err, ErrPayloadVoteInvalid) {
		t.Fatalf("payload build issue=%v, want ErrPayloadVoteInvalid", err)
	} else if !IsExpectedSkip(err) {
		t.Fatalf("already-justified vote was not marked expected: %v", err)
	}

	farState, err := workingState.Clone()
	if err != nil {
		t.Fatalf("clone working state: %v", err)
	}
	farRoot := [32]byte{0x77}
	for len(farState.HistoricalBlockHashes) <= 7 {
		farState.HistoricalBlockHashes = append(farState.HistoricalBlockHashes, make([]byte, types.RootSize))
	}
	farState.HistoricalBlockHashes[7] = copyRoot(farRoot)
	// Grow the justification window alongside the chain so slot 7 is in range and
	// the vote is rejected as not-justifiable rather than out-of-range.
	farState.JustifiedSlots = types.BitlistExtend(farState.JustifiedSlots, 7)

	farTarget := *data
	farTarget.Target = &types.Checkpoint{Slot: 7, Root: farRoot}
	payload.Data = &farTarget
	if err := payloadBuildIssue(farState, knownRoots, payload); !errors.Is(err, ErrPayloadVoteInvalid) {
		t.Fatalf("payload build issue=%v, want ErrPayloadVoteInvalid", err)
	} else if !IsExpectedSkip(err) {
		t.Fatalf("not-justifiable vote was not marked expected: %v", err)
	}
}

func TestPayloadBuildIssueSkipsStaleSource(t *testing.T) {
	headState, parentRoot, data, dataRoot := postHeaderVoteInput(t)
	workingState, err := transitionBlock(headState, 3, newBlock(3, 0, parentRoot, nil))
	if err != nil {
		t.Fatalf("transition header: %v", err)
	}

	// workingState is justified at slot 1; a vote sourcing at slot 0 no longer
	// matches the current justified checkpoint and is an expected skip.
	staleSource := *data
	staleSource.Source = &types.Checkpoint{Slot: 0, Root: data.Head.Root}
	payload := AttestationPayload{
		DataRoot: dataRoot,
		Data:     &staleSource,
		Proofs:   []*types.SingleMessageAggregate{mockProof([]uint64{0})},
	}
	err = payloadBuildIssue(workingState, RootSet{parentRoot: true}, payload)
	if !errors.Is(err, ErrPayloadSourceNotCurrentJustified) {
		t.Fatalf("payload build issue=%v, want ErrPayloadSourceNotCurrentJustified", err)
	}
	if !IsExpectedSkip(err) {
		t.Fatalf("stale source skip was not marked expected: %v", err)
	}
}

func TestPayloadBuildIssueAllowsGenesisSelfVote(t *testing.T) {
	root := [32]byte{1}
	state := &types.State{
		LatestFinalized:       &types.Checkpoint{Root: root},
		HistoricalBlockHashes: [][]byte{append([]byte(nil), root[:]...)},
		JustifiedSlots:        types.NewBitlistSSZ(0),
	}
	data := &types.AttestationData{
		Head:   &types.Checkpoint{Root: root},
		Source: &types.Checkpoint{Root: root},
		Target: &types.Checkpoint{Root: root},
	}
	dataRoot := hashAttestationData(t, data)
	err := payloadBuildIssue(state, RootSet{root: true}, AttestationPayload{
		DataRoot: dataRoot,
		Data:     data,
		Proofs:   []*types.SingleMessageAggregate{mockProof([]uint64{0})},
	})
	if err != nil {
		t.Fatalf("genesis self-vote rejected: %v", err)
	}
}
