// Package p2p implements networking for the Lean Ethereum consensus protocol.
package p2p

// Gossipsub topic names (Devnet 0)
const (
	TopicBlocks       = "/leanconsensus/devnet0/block/ssz_snappy"
	TopicAttestations = "/leanconsensus/devnet0/attestation/ssz_snappy"
)
