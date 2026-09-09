package metrics

import "strings"

const unknownLabel = "unknown"
const maxLabelRunes = 64

var syncStatusLabels = []string{"idle", "syncing", "synced", unknownLabel}

const (
	AggregatorSkipNotAggregator = "not_aggregator"
	AggregatorSkipMissingState  = "missing_state"
	AggregatorSkipSpawnFailed   = "spawn_failed"
	AggregatorSkipOther         = "other"
)

var aggregatorSkipReasons = []string{
	AggregatorSkipNotAggregator,
	AggregatorSkipMissingState,
	AggregatorSkipSpawnFailed,
	AggregatorSkipOther,
}

// Reasons an individual aggregation group is dropped inside a session. A
// session that drops every group reports produced=0, which is otherwise
// indistinguishable from having nothing to aggregate.
const (
	AggGroupSkipTargetJustified = "target_justified"
	AggGroupSkipTooFewSigners   = "too_few_signers"
	AggGroupSkipBudget          = "budget"
	AggGroupSkipSessionCap      = "session_cap"
	AggGroupSkipError           = "error"
)

var aggregationGroupSkipReasons = []string{
	AggGroupSkipTargetJustified,
	AggGroupSkipTooFewSigners,
	AggGroupSkipBudget,
	AggGroupSkipSessionCap,
	AggGroupSkipError,
}

// Attestation-aggregate coverage label values, matching the leanSpec reference
// node's section/subnet/direction taxonomy.
const (
	CoverageSectionTimely            = "timely"
	CoverageSectionLate              = "late"
	CoverageSectionBlock             = "block"
	CoverageSectionCombined          = "combined"
	CoverageSectionAggregateStartNew = "aggregate_start_new"
	CoverageSectionProposalPayloads  = "proposal_payloads"
	CoverageSectionProposalGossip    = "proposal_gossip"
	CoverageSectionProposalCombined  = "proposal_combined"

	// CoverageSubnetCombined is the all-subnet validator total for a section.
	CoverageSubnetCombined = "combined"

	CoverageDiffBlockOnly  = "block_only"
	CoverageDiffTimelyOnly = "timely_only"
)

func syncStatusLabel(status string) string {
	status = labelOrUnknown(status)
	for _, label := range syncStatusLabels {
		if status == label {
			return status
		}
	}
	return unknownLabel
}

func aggregatorSkipReason(reason string) string {
	reason = labelOrUnknown(reason)
	for _, allowed := range aggregatorSkipReasons {
		if reason == allowed {
			return reason
		}
	}
	return AggregatorSkipOther
}

func aggregationGroupSkipReason(reason string) string {
	reason = labelOrUnknown(reason)
	for _, allowed := range aggregationGroupSkipReasons {
		if reason == allowed {
			return reason
		}
	}
	return AggGroupSkipError
}

func labelOrUnknown(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return unknownLabel
	}
	return truncateLabel(label, maxLabelRunes)
}

func truncateLabel(label string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	count := 0
	for idx := range label {
		if count == maxRunes {
			return label[:idx]
		}
		count++
	}
	return label
}
