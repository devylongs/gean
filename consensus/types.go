// Package consensus implements the Lean Ethereum consensus protocol (Devnet 0).
package consensus

//go:generate go run github.com/ferranbt/fastssz/sszgen --path=. --objs=Checkpoint,Config,Vote,SignedVote,BlockHeader,BlockBody,Block,SignedBlock,State

// Primitive Types

type Slot uint64
type ValidatorIndex uint64
type Root [32]byte

func (r Root) IsZero() bool { return r == Root{} }

// Constants

const (
	SecondsPerSlot         uint64 = 4
	IntervalsPerSlot       uint64 = 4
	SecondsPerInterval     uint64 = SecondsPerSlot / IntervalsPerSlot
	HistoricalRootsLimit   uint64 = 1 << 18 // 262144
	ValidatorRegistryLimit uint64 = 1 << 12 // 4096
)

// Time Helpers

func (s Slot) Time(genesisTime uint64) uint64 {
	return genesisTime + uint64(s)*SecondsPerSlot
}

func SlotAt(time, genesisTime uint64) Slot {
	if time < genesisTime {
		return 0
	}
	return Slot((time - genesisTime) / SecondsPerSlot)
}

func IntervalAt(time, genesisTime uint64) uint64 {
	if time < genesisTime {
		return 0
	}
	offset := (time - genesisTime) % SecondsPerSlot
	return offset / SecondsPerInterval
}

// SSZ Containers

// Checkpoint is a (root, slot) pair identifying a block in the chain.
type Checkpoint struct {
	Root Root `ssz-size:"32"`
	Slot Slot
}

type Config struct {
	NumValidators uint64
	GenesisTime   uint64
}

// Vote is a validator's attestation for head, target, and source.
type Vote struct {
	ValidatorID uint64
	Slot        Slot
	Head        Checkpoint
	Target      Checkpoint
	Source      Checkpoint
}

type SignedVote struct {
	Data      Vote
	Signature Root `ssz-size:"32"`
}

type BlockHeader struct {
	Slot          Slot
	ProposerIndex uint64
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	BodyRoot      Root `ssz-size:"32"`
}

type BlockBody struct {
	Attestations []SignedVote `ssz-max:"4096"`
}

type Block struct {
	Slot          Slot
	ProposerIndex uint64
	ParentRoot    Root `ssz-size:"32"`
	StateRoot     Root `ssz-size:"32"`
	Body          BlockBody
}

type SignedBlock struct {
	Message   Block
	Signature Root `ssz-size:"32"`
}

// State is the main consensus state object.
type State struct {
	Config            Config
	Slot              Slot
	LatestBlockHeader BlockHeader

	LatestJustified Checkpoint
	LatestFinalized Checkpoint

	HistoricalBlockHashes []Root `ssz-max:"262144" ssz-size:"?,32"`
	JustifiedSlots        []byte `ssz-max:"32768"`

	JustificationRoots      []Root `ssz-max:"262144" ssz-size:"?,32"`
	JustificationValidators []byte `ssz-max:"1073741824"`
}
