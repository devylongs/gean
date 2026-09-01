package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func TestCoverageSetCountsDistinctValidatorsAndSubnets(t *testing.T) {
	tests := []struct {
		name           string
		validatorCount int
		committeeCount uint64
		participants   [][]uint64
		wantValidators int
		wantSubnets    int
	}{
		{
			name: "empty", validatorCount: 8, committeeCount: 4,
			wantValidators: 0, wantSubnets: 0,
		},
		{
			name: "single subnet", validatorCount: 8, committeeCount: 4,
			participants:   [][]uint64{{0, 4}},
			wantValidators: 2, wantSubnets: 1,
		},
		{
			name: "overlapping proofs count each validator once", validatorCount: 8, committeeCount: 4,
			participants:   [][]uint64{{1, 2}, {2, 3}},
			wantValidators: 3, wantSubnets: 3,
		},
		{
			name: "all subnets", validatorCount: 8, committeeCount: 4,
			participants:   [][]uint64{{0, 1, 2, 3}},
			wantValidators: 4, wantSubnets: 4,
		},
		{
			// Out-of-range ids must not panic or inflate the count; a peer can
			// send bits wider than our registry.
			name: "ids beyond the registry are ignored", validatorCount: 4, committeeCount: 2,
			participants:   [][]uint64{{0, 99}},
			wantValidators: 1, wantSubnets: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := newCoverageSet(tt.validatorCount, tt.committeeCount)
			for _, ids := range tt.participants {
				set.add(types.BitlistFromIndices(ids))
			}

			gotValidators := 0
			for _, seen := range set.seen {
				if seen {
					gotValidators++
				}
			}
			gotSubnets := 0
			for _, has := range set.hasSubnet {
				if has {
					gotSubnets++
				}
			}
			if gotValidators != tt.wantValidators {
				t.Errorf("validators=%d, want %d", gotValidators, tt.wantValidators)
			}
			if gotSubnets != tt.wantSubnets {
				t.Errorf("subnets=%d, want %d", gotSubnets, tt.wantSubnets)
			}
		})
	}
}

func TestNewCoverageSetRejectsDegenerateShapes(t *testing.T) {
	if set := newCoverageSet(0, 4); set != nil {
		t.Error("no validators: want nil set")
	}
	if set := newCoverageSet(8, 0); set != nil {
		t.Error("no committees: want nil set (subnet is vid % committeeCount)")
	}
	// A nil set must absorb both calls rather than panic.
	var nilSet *coverageSet
	nilSet.add(types.BitlistFromIndices([]uint64{1}))
	nilSet.record("timely")
}

func TestCoverageSetOrUnionsBothDimensions(t *testing.T) {
	a := newCoverageSet(8, 4)
	a.add(types.BitlistFromIndices([]uint64{0}))
	b := newCoverageSet(8, 4)
	b.add(types.BitlistFromIndices([]uint64{5}))

	a.or(b)

	if !a.seen[0] || !a.seen[5] {
		t.Error("union lost a validator")
	}
	if !a.hasSubnet[0] || !a.hasSubnet[1] {
		t.Errorf("union lost a subnet: %v", a.hasSubnet)
	}
	// or(nil) is a no-op, not a panic.
	a.or(nil)
}

func TestSnapshotNewPayloadParticipantsEmptyIsNil(t *testing.T) {
	s := makeTestStore()
	if got := snapshotNewPayloadParticipants(s); got != nil {
		t.Errorf("empty buffer snapshot=%v, want nil so the caller keeps its last round", got)
	}
}

func TestSnapshotNewPayloadParticipantsGroupsBySlot(t *testing.T) {
	s := makeTestStore()
	for _, tc := range []struct {
		root byte
		slot uint64
		ids  []uint64
	}{
		{root: 1, slot: 7, ids: []uint64{0, 1}},
		{root: 2, slot: 7, ids: []uint64{2}},
		{root: 3, slot: 8, ids: []uint64{3}},
	} {
		var dr [32]byte
		dr[0] = tc.root
		s.NewPayloads.Push(dr,
			&types.AttestationData{Slot: tc.slot, Target: &types.Checkpoint{}},
			&types.SingleMessageAggregate{Participants: types.BitlistFromIndices(tc.ids), Proof: []byte{0x01}},
		)
	}

	got := snapshotNewPayloadParticipants(s)

	if len(got[7]) != 2 {
		t.Errorf("slot 7 proofs=%d, want 2", len(got[7]))
	}
	if len(got[8]) != 1 {
		t.Errorf("slot 8 proofs=%d, want 1", len(got[8]))
	}
	if _, ok := got[9]; ok {
		t.Error("slot 9 present, want absent")
	}
}
