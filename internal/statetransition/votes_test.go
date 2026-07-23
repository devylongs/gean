package statetransition

import (
	"errors"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestVoteInvalidReason(t *testing.T) {
	const numValidators = 4

	roots := make([][types.RootSize]byte, 9)
	hashes := make([][]byte, len(roots))
	for i := range roots {
		roots[i][0] = byte(i + 1)
		hashes[i] = make([]byte, types.RootSize)
		copy(hashes[i], roots[i][:])
	}

	newState := func() *types.State {
		s := makeGenesisState(numValidators)
		s.LatestFinalized = &types.Checkpoint{Slot: 0, Root: roots[0]}
		s.HistoricalBlockHashes = hashes
		// Track slots 1..8 (all unjustified). Header processing sizes the window
		// to (finalized, block.slot-1], so a vote referencing a slot beyond it is
		// out of range; the cases below stay within the tracked window unless they
		// explicitly probe the out-of-range rejection.
		s.JustifiedSlots = types.NewBitlistSSZ(8)
		return s
	}
	cp := func(slot uint64, root [types.RootSize]byte) *types.Checkpoint {
		return &types.Checkpoint{Slot: slot, Root: root}
	}

	cases := []struct {
		name    string
		setup   func(s *types.State)
		source  *types.Checkpoint
		target  *types.Checkpoint
		want    string
		wantErr bool
	}{
		{
			name:   "valid",
			source: cp(0, roots[0]),
			target: cp(1, roots[1]),
			want:   "",
		},
		{
			name:   "nil_input",
			source: nil,
			target: cp(1, roots[1]),
			want:   VoteReasonNilInput,
		},
		{
			name:   "source_not_justified",
			source: cp(2, roots[2]),
			target: cp(3, roots[3]),
			want:   VoteReasonSourceNotJustified,
		},
		{
			name:   "target_already_justified",
			setup:  func(s *types.State) { setSlotJustified(s, 0, 1) },
			source: cp(0, roots[0]),
			target: cp(1, roots[1]),
			want:   VoteReasonTargetAlreadyJustified,
		},
		{
			name:   "zero_root",
			source: cp(0, [types.RootSize]byte{}),
			target: cp(1, roots[1]),
			want:   VoteReasonZeroRoot,
		},
		{
			name:   "chain_mismatch",
			source: cp(0, roots[0]),
			target: cp(1, roots[2]),
			want:   VoteReasonChainMismatch,
		},
		{
			// leanSpec checks target justification before chain membership, so an
			// already-justified target is skipped before the chain mismatch is seen.
			name:   "already_justified_takes_priority_over_chain_mismatch",
			setup:  func(s *types.State) { setSlotJustified(s, 0, 1) },
			source: cp(0, roots[0]),
			target: cp(1, roots[2]),
			want:   VoteReasonTargetAlreadyJustified,
		},
		{
			name:   "target_not_after_source",
			setup:  func(s *types.State) { setSlotJustified(s, 0, 2) },
			source: cp(2, roots[2]),
			target: cp(1, roots[1]),
			want:   VoteReasonTargetNotAfterSource,
		},
		{
			name:   "target_not_justifiable",
			source: cp(0, roots[0]),
			target: cp(7, roots[7]),
			want:   VoteReasonTargetNotJustifiable,
		},
		{
			// Source slot past the tracked window is a hard block rejection, not a
			// silently dropped vote. The source root is irrelevant: the justification
			// query raises before any chain check runs.
			name:    "source_slot_out_of_range",
			source:  cp(9, roots[0]),
			target:  cp(1, roots[1]),
			wantErr: true,
		},
		{
			// Target slot past the tracked window likewise rejects the block, after
			// the in-range source clears its justification query.
			name:    "target_slot_out_of_range",
			source:  cp(0, roots[0]),
			target:  cp(9, roots[1]),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newState()
			if tc.setup != nil {
				tc.setup(s)
			}
			got, err := VoteInvalidReason(s, tc.source, tc.target)
			if tc.wantErr {
				var oorErr *JustifiedSlotOutOfRangeError
				if !errors.As(err, &oorErr) {
					t.Fatalf("VoteInvalidReason err = %v, want JustifiedSlotOutOfRangeError", err)
				}
				if IsValidVote(s, tc.source, tc.target) {
					t.Fatal("IsValidVote must be false on out-of-range rejection")
				}
				return
			}
			if err != nil {
				t.Fatalf("VoteInvalidReason unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("VoteInvalidReason = %q, want %q", got, tc.want)
			}
			if valid := IsValidVote(s, tc.source, tc.target); valid != (got == "") {
				t.Fatalf("IsValidVote=%v but reason=%q; they must agree", valid, got)
			}
		})
	}

	t.Run("nil_state", func(t *testing.T) {
		if got, err := VoteInvalidReason(nil, cp(0, roots[0]), cp(1, roots[1])); err != nil || got != VoteReasonNilInput {
			t.Fatalf("VoteInvalidReason(nil state) = %q (err %v), want %q", got, err, VoteReasonNilInput)
		}
		if IsValidVote(nil, cp(0, roots[0]), cp(1, roots[1])) {
			t.Fatalf("IsValidVote(nil state) = true, want false")
		}
	})
}

func TestHeadMatchesChain(t *testing.T) {
	var onChain, offChain [types.RootSize]byte
	onChain[0] = 0x11
	offChain[0] = 0x22

	hashes := make([][]byte, 3)
	for i := range hashes {
		hashes[i] = make([]byte, types.RootSize)
	}
	copy(hashes[2], onChain[:])

	state := makeGenesisState(1)
	state.HistoricalBlockHashes = hashes

	cp := func(slot uint64, root [types.RootSize]byte) *types.Checkpoint {
		return &types.Checkpoint{Slot: slot, Root: root}
	}

	cases := []struct {
		name string
		head *types.Checkpoint
		want bool
	}{
		{"on_chain", cp(2, onChain), true},
		{"zero_root", cp(2, [types.RootSize]byte{}), false},
		{"off_chain", cp(2, offChain), false},
		{"beyond_chain", cp(5, onChain), false},
		{"nil", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := HeadMatchesChain(state, tc.head); got != tc.want {
				t.Fatalf("HeadMatchesChain(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
