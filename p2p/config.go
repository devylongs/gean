// Package p2p implements networking for the Lean Ethereum consensus protocol.
package p2p

// Network constants
const (
	MaxRequestBlocks = 1 << 10 // 1024
)

// Domain types for message ID isolation
var (
	MessageDomainInvalidSnappy = [4]byte{0x00, 0x00, 0x00, 0x00}
	MessageDomainValidSnappy   = [4]byte{0x01, 0x00, 0x00, 0x00}
)

// Protocol IDs
const (
	StatusProtocolV1       = "/leanconsensus/req/status/1/"
	BlocksByRootProtocolV1 = "/leanconsensus/req/blocks_by_root/1/"
)
