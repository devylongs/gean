package blockbuilder

import (
	"github.com/geanlabs/gean/internal/attestationproof"
	"github.com/geanlabs/gean/internal/types"
)

type AttestationPayload struct {
	DataRoot [32]byte
	Data     *types.AttestationData
	Proofs   []*types.SingleMessageAggregate
}

// KnownRoots answers whether a block root is one this node has stored.
//
// The builder only ever asks membership, a handful of times per proposal, so
// this is a predicate rather than a set. It used to be a map that the proposal
// path filled by scanning every key in TableBlockHeaders and allocating an
// entry per root — tens of thousands of them on a mature chain, rebuilt for
// every block produced, on the tick loop.
type KnownRoots interface {
	Contains(root [32]byte) bool
}

// RootSet is the in-memory implementation, used by tests and by any caller that
// genuinely holds the whole set already.
type RootSet map[[32]byte]bool

func (roots RootSet) Contains(root [32]byte) bool {
	return roots[root]
}

// KnownRootsFunc adapts a plain lookup — a point read against storage, say —
// into a KnownRoots.
type KnownRootsFunc func(root [32]byte) bool

func (f KnownRootsFunc) Contains(root [32]byte) bool {
	return f(root)
}

type Input struct {
	HeadState       *types.State
	Slot            uint64
	ProposerIndex   uint64
	ParentRoot      [32]byte
	KnownBlockRoots KnownRoots
	Payloads        []AttestationPayload
	ProofMerger     attestationproof.MergeProvider
}

type Result struct {
	Block             *types.Block
	AttestationProofs []*types.SingleMessageAggregate
	PayloadErrors     []PayloadError
}

type PayloadError struct {
	DataRoot [32]byte
	Err      error
}
