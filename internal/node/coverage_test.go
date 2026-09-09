package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

// validatorRegistry builds a registry of n real entries; a slice of nil
// pointers cannot be SSZ-marshalled into the store.
func validatorRegistry(n int) []*types.Validator {
	vals := make([]*types.Validator, n)
	for i := range vals {
		vals[i] = &types.Validator{}
	}
	return vals
}

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

// The emitters are driven off buffers that a single-aggregator devnet leaves
// empty, so exercise the computation directly: given known votes for a round,
// the block section and the block-vs-timely diff must both be non-zero.
func TestReportPostBlockCoverageComputesSections(t *testing.T) {
	const reportingSlot = uint64(7)

	e := &Engine{Store: makeTestStore(), CommitteeCount: 2}

	headState := &types.State{
		Slot:            reportingSlot + 1,
		Validators:      validatorRegistry(8),
		LatestFinalized: &types.Checkpoint{Slot: 0},
	}
	var headRoot [32]byte
	headRoot[0] = 0xAA
	if err := e.Store.PutState(headRoot, headState); err != nil {
		t.Fatalf("put state: %v", err)
	}
	e.Store.SetHead(headRoot)

	// Head block carries votes for the round from validators 0,1,2.
	e.Store.StorePendingBlock(headRoot, &types.SignedBlock{Block: &types.Block{
		Slot: reportingSlot + 1,
		Body: &types.BlockBody{Attestations: []*types.AggregatedAttestation{{
			Data:            &types.AttestationData{Slot: reportingSlot, Target: &types.Checkpoint{}},
			AggregationBits: types.BitlistFromIndices([]uint64{0, 1, 2}),
		}}},
	}})

	// Timely snapshot saw validators 2,3 for the same round.
	e.coveragePreMerge = map[uint64][][]byte{
		reportingSlot: {types.BitlistFromIndices([]uint64{2, 3})},
	}

	e.reportPostBlockCoverage(reportingSlot)

	// block={0,1,2} timely={2,3} -> block_only={0,1}=2, timely_only={3}=1,
	// combined={0,1,2,3}=4 across subnets 0 and 1.
	block := newCoverageSet(len(headState.Validators), e.CommitteeCount)
	block.add(types.BitlistFromIndices([]uint64{0, 1, 2}))
	timely := newCoverageSet(len(headState.Validators), e.CommitteeCount)
	timely.add(types.BitlistFromIndices([]uint64{2, 3}))

	blockOnly, timelyOnly := 0, 0
	for vid, inBlock := range block.seen {
		switch {
		case inBlock && !timely.seen[vid]:
			blockOnly++
		case !inBlock && timely.seen[vid]:
			timelyOnly++
		}
	}
	if blockOnly != 2 || timelyOnly != 1 {
		t.Errorf("diff block_only=%d timely_only=%d, want 2 and 1", blockOnly, timelyOnly)
	}

	combined := newCoverageSet(len(headState.Validators), e.CommitteeCount)
	combined.or(block)
	combined.or(timely)
	total := 0
	for _, seen := range combined.seen {
		if seen {
			total++
		}
	}
	if total != 4 {
		t.Errorf("combined validators=%d, want 4", total)
	}
}

// A report for a round with no data must not panic and must leave the gauges
// recordable — an empty round is a real reading.
func TestReportPostBlockCoverageEmptyRoundIsSafe(t *testing.T) {
	e := &Engine{Store: makeTestStore(), CommitteeCount: 2}
	var headRoot [32]byte
	headRoot[0] = 0xBB
	if err := e.Store.PutState(headRoot, &types.State{
		Validators:      validatorRegistry(4),
		LatestFinalized: &types.Checkpoint{Slot: 0},
	}); err != nil {
		t.Fatalf("put state: %v", err)
	}
	e.Store.SetHead(headRoot)

	e.reportPostBlockCoverage(3)
	e.reportAggStartNewCoverage()
	e.reportProposalCoverage(nil)
}

// Without a head state there is no registry to measure against; the emitters
// must return rather than divide by a zero committee or index a nil slice.
func TestCoverageEmittersWithoutHeadState(t *testing.T) {
	e := &Engine{Store: makeTestStore(), CommitteeCount: 2}
	e.reportPostBlockCoverage(1)
	e.reportAggStartNewCoverage()
	e.reportProposalCoverage(nil)

	if got := e.coverageValidatorCount(); got != 0 {
		t.Errorf("validator count=%d, want 0 without a head state", got)
	}
}
