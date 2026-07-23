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

type KnownRoots map[[32]byte]bool

func (roots KnownRoots) Contains(root [32]byte) bool {
	return roots[root]
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
