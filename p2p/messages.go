package p2p

import "github.com/devylongs/gean/consensus"

// Status is the handshake message sent upon new connections.
type Status struct {
	Finalized consensus.Checkpoint
	Head      consensus.Checkpoint
}

// BlocksByRootRequest is a request for blocks by their root hashes.
type BlocksByRootRequest []consensus.Root

// BlocksByRootResponse is a response containing signed blocks.
type BlocksByRootResponse []consensus.SignedBlock
