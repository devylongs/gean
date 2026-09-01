package node

import (
	"strconv"

	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// Attestation-aggregate coverage reporting. Pure observability: nothing here
// feeds fork choice or the state transition. Sections and subnet labels follow
// the shared devnet dashboard's taxonomy so gean's numbers line up with the
// other clients' on the same panels.
//
// Subnet assignment is validator_id % committee_count, matching p2p.SubnetID.

// coverageSet accumulates the distinct validators seen through one channel, and
// which subnets they fall in.
type coverageSet struct {
	seen      []bool
	hasSubnet []bool
}

func newCoverageSet(validatorCount int, committeeCount uint64) *coverageSet {
	if validatorCount <= 0 || committeeCount == 0 {
		return nil
	}
	return &coverageSet{
		seen:      make([]bool, validatorCount),
		hasSubnet: make([]bool, committeeCount),
	}
}

func (c *coverageSet) add(participants []byte) {
	if c == nil {
		return
	}
	committeeCount := uint64(len(c.hasSubnet))
	for _, vid := range types.BitlistIndices(participants) {
		if vid >= uint64(len(c.seen)) {
			continue
		}
		c.seen[vid] = true
		c.hasSubnet[vid%committeeCount] = true
	}
}

func (c *coverageSet) or(other *coverageSet) {
	if c == nil || other == nil {
		return
	}
	for i, v := range other.seen {
		if v {
			c.seen[i] = true
		}
	}
	for i, v := range other.hasSubnet {
		if v {
			c.hasSubnet[i] = true
		}
	}
}

// record publishes one section: the combined validator count, the per-subnet
// split, and how many subnets were covered at all.
func (c *coverageSet) record(section string) {
	if c == nil {
		return
	}
	committeeCount := len(c.hasSubnet)
	total := 0
	subnetCounts := make([]int, committeeCount)
	for vid, wasSeen := range c.seen {
		if !wasSeen {
			continue
		}
		total++
		subnetCounts[vid%committeeCount]++
	}
	metrics.SetAttestationAggregateCoverageValidators(section, metrics.CoverageSubnetCombined, total)
	for subnet, count := range subnetCounts {
		metrics.SetAttestationAggregateCoverageValidators(section, "subnet_"+strconv.Itoa(subnet), count)
	}
	covered := 0
	for _, has := range c.hasSubnet {
		if has {
			covered++
		}
	}
	metrics.SetAttestationAggregateCoverageSubnets(section, covered)
}

// validatorCount reads the head state's registry size, which is what every
// coverage section is measured against.
func (e *Engine) coverageValidatorCount() int {
	headState := e.Store.GetState(e.Store.Head())
	if headState == nil {
		return 0
	}
	return len(headState.Validators)
}

// snapshotNewPayloadParticipants captures the participant bits currently in the
// new-payload buffer, tagged by the slot each vote is for. Taken before the tick
// promotes new payloads into known, so the "timely" section reflects what had
// arrived by the promotion boundary rather than what survived it.
//
// Returns nil when the buffer is empty so the caller can keep its previous
// snapshot: a node that saw nothing this tick should still report the round it
// last observed.
func snapshotNewPayloadParticipants(s *store.ConsensusStore) map[uint64][][]byte {
	entries := s.NewPayloads.Entries()
	if len(entries) == 0 {
		return nil
	}
	bySlot := make(map[uint64][][]byte, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Data == nil {
			continue
		}
		for _, proof := range entry.Proofs {
			if proof != nil {
				bySlot[entry.Data.Slot] = append(bySlot[entry.Data.Slot], proof.Participants)
			}
		}
	}
	if len(bySlot) == 0 {
		return nil
	}
	return bySlot
}

// reportPostBlockCoverage emits the timely / late / block / combined sections
// for reportingSlot, plus the block-vs-timely symmetric difference. Every
// section is the same cohort — validators whose votes *for* reportingSlot were
// seen — counted through a different channel.
func (e *Engine) reportPostBlockCoverage(reportingSlot uint64) {
	validatorCount := e.coverageValidatorCount()
	if validatorCount == 0 || e.CommitteeCount == 0 {
		return
	}

	timely := newCoverageSet(validatorCount, e.CommitteeCount)
	late := newCoverageSet(validatorCount, e.CommitteeCount)
	block := newCoverageSet(validatorCount, e.CommitteeCount)

	for _, participants := range e.coveragePreMerge[reportingSlot] {
		timely.add(participants)
	}
	for _, participants := range snapshotNewPayloadParticipants(e.Store)[reportingSlot] {
		late.add(participants)
	}
	if head := e.Store.GetSignedBlock(e.Store.Head()); head != nil && head.Block != nil && head.Block.Body != nil {
		for _, att := range head.Block.Body.Attestations {
			if att != nil && att.Data != nil && att.Data.Slot == reportingSlot {
				block.add(att.AggregationBits)
			}
		}
	}

	combined := newCoverageSet(validatorCount, e.CommitteeCount)
	combined.or(timely)
	combined.or(late)
	combined.or(block)

	// A genuine all-zero reading is real information — an empty slot, or a
	// proposer that dropped every attestation — and reads as a dip rather than
	// a gauge silently holding its last value. Always record the four sections.
	timely.record(metrics.CoverageSectionTimely)
	late.record(metrics.CoverageSectionLate)
	block.record(metrics.CoverageSectionBlock)
	combined.record(metrics.CoverageSectionCombined)

	// The diff is only meaningful once a block has actually reported this round.
	// Without that guard a missed slot reports block_only=0, timely_only=N,
	// which reads as the proposer dropping votes it never had the chance to
	// include. Here there is no genuine zero to fall back on, so leave the
	// previous value standing.
	blockSawAny := false
	for _, v := range block.seen {
		if v {
			blockSawAny = true
			break
		}
	}
	if !blockSawAny {
		return
	}
	blockOnly, timelyOnly := 0, 0
	for vid, inBlock := range block.seen {
		switch {
		case inBlock && !timely.seen[vid]:
			blockOnly++
		case !inBlock && timely.seen[vid]:
			timelyOnly++
		}
	}
	metrics.SetAttestationAggregateCoverageDiffValidators(metrics.CoverageDiffBlockOnly, blockOnly)
	metrics.SetAttestationAggregateCoverageDiffValidators(metrics.CoverageDiffTimelyOnly, timelyOnly)
}

// reportAggStartNewCoverage emits what the aggregation session is about to work
// from, recorded at interval 2 just before dispatch.
func (e *Engine) reportAggStartNewCoverage() {
	validatorCount := e.coverageValidatorCount()
	if validatorCount == 0 || e.CommitteeCount == 0 {
		return
	}
	set := newCoverageSet(validatorCount, e.CommitteeCount)
	for _, bySlot := range snapshotNewPayloadParticipants(e.Store) {
		for _, participants := range bySlot {
			set.add(participants)
		}
	}
	set.record(metrics.CoverageSectionAggregateStartNew)
}

// reportProposalCoverage emits the validators covered by the aggregates we are
// about to publish in our own block.
func (e *Engine) reportProposalCoverage(attestations []*types.AggregatedAttestation) {
	validatorCount := e.coverageValidatorCount()
	if validatorCount == 0 || e.CommitteeCount == 0 {
		return
	}
	set := newCoverageSet(validatorCount, e.CommitteeCount)
	for _, att := range attestations {
		if att != nil {
			set.add(att.AggregationBits)
		}
	}
	set.record(metrics.CoverageSectionProposalCombined)
}
