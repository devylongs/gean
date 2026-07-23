package types

const (
	SecondsPerSlot          = 4
	IntervalsPerSlot        = 5
	MillisecondsPerSlot     = SecondsPerSlot * 1000
	MillisecondsPerInterval = MillisecondsPerSlot / IntervalsPerSlot

	HistoricalRootsLimit   = 1 << 18
	ValidatorRegistryLimit = 1 << 12
	// AttestationCommitteeCount is the default number of attestation subnets.
	// It is a network parameter, not a fixed invariant: config.yaml's
	// ATTESTATION_COMMITTEE_COUNT or --attestation-committee-count override it.
	AttestationCommitteeCount  = 1
	JustificationLookbackSlots = 3
	MaxAttestationsData        = 8

	JustificationValidatorsLimit = HistoricalRootsLimit * ValidatorRegistryLimit

	PubkeySize        = 52
	SignatureSize     = 2536
	RootSize          = 32
	ByteList512KiBMax = 1 << 19

	SyncToleranceSlots = 2

	GossipDisparityIntervals = 1

	MaxRequestBlocks = 1 << 10

	MinSlotsForBlockRequests = 3600
)
