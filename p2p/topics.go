package p2p

// Gossip topic names for the lean consensus protocol.
const (
	TopicBlocks       = "/leanconsensus/blocks/ssz_snappy"
	TopicAttestations = "/leanconsensus/attestations/ssz_snappy"
)

// TopicEncoding specifies the encoding used for gossip messages.
const TopicEncoding = "ssz_snappy"
