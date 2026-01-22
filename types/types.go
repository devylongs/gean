package types

//go:generate sszgen --path=. --objs=Checkpoint,Config,Vote,SignedVote,BlockHeader,BlockBody,Block,SignedBlock,State

type Slot uint64
type ValidatorIndex uint64
type Root [32]byte
type Bytes32 = Root
type Bytes48 [48]byte
type Bytes96 [96]byte

const (
	SecondsPerSlot         uint64 = 4
	IntervalsPerSlot       uint64 = 4
	SecondsPerInterval     uint64 = SecondsPerSlot / IntervalsPerSlot
	HistoricalRootsLimit   uint64 = 262144 // 2^18
	ValidatorRegistryLimit uint64 = 4096   // 2^12
)

func (r Root) IsZero() bool {
	return r == Root{}
}

func SlotToTime(slot Slot, genesisTime uint64) uint64 {
	return genesisTime + uint64(slot)*SecondsPerSlot
}

func TimeToSlot(time, genesisTime uint64) Slot {
	if time < genesisTime {
		return 0
	}
	return Slot((time - genesisTime) / SecondsPerSlot)
}

// IsJustifiableAfter checks if this slot is a valid justification target.
//
// According to 3SF-mini, a slot is justifiable if delta from finalized is:
//   - <= 5 (first few slots always justifiable)
//   - A perfect square (9, 16, 25, 36...)
//   - A pronic number n*(n+1): 6, 12, 20, 30...
//
// See: https://github.com/ethereum/research/blob/master/3sf-mini/consensus.py
func (s Slot) IsJustifiableAfter(finalizedSlot Slot) bool {
	if s < finalizedSlot {
		return false
	}
	delta := uint64(s - finalizedSlot)

	// Rule 1: first few slots always justifiable
	if delta <= 5 {
		return true
	}
	// Rule 2: perfect square
	if isPerfectSquare(delta) {
		return true
	}
	// Rule 3: pronic number (n is pronic if 4n+1 is a perfect square)
	if isPerfectSquare(4*delta + 1) {
		return true
	}
	return false
}

func isPerfectSquare(n uint64) bool {
	if n == 0 {
		return true
	}
	root := isqrt(n)
	return root*root == n
}

func isqrt(n uint64) uint64 {
	if n == 0 {
		return 0
	}
	x := n
	y := (x + 1) / 2
	for y < x {
		x = y
		y = (x + n/x) / 2
	}
	return x
}

// IsProposer checks if validator is the proposer for the slot (round-robin).
func IsProposer(validatorIndex ValidatorIndex, slot Slot, numValidators uint64) bool {
	if numValidators == 0 {
		return false
	}
	return uint64(slot)%numValidators == uint64(validatorIndex)
}

type Checkpoint struct {
	Root Root `ssz-size:"32"`
	Slot Slot `ssz-size:"8"`
}

type Config struct {
	NumValidators uint64 `ssz-size:"8"`
	GenesisTime   uint64 `ssz-size:"8"`
}

type Vote struct {
	ValidatorID uint64 `ssz-size:"8"`
	Slot        Slot   `ssz-size:"8"`
	Head        Checkpoint
	Target      Checkpoint
	Source      Checkpoint
}

type SignedVote struct {
	Data      Vote
	Signature Bytes32 `ssz-size:"32"`
}

type BlockHeader struct {
	Slot          Slot   `ssz-size:"8"`
	ProposerIndex uint64 `ssz-size:"8"`
	ParentRoot    Root   `ssz-size:"32"`
	StateRoot     Root   `ssz-size:"32"`
	BodyRoot      Root   `ssz-size:"32"`
}

type BlockBody struct {
	Attestations []SignedVote `ssz-max:"4096"`
}

type Block struct {
	Slot          Slot   `ssz-size:"8"`
	ProposerIndex uint64 `ssz-size:"8"`
	ParentRoot    Root   `ssz-size:"32"`
	StateRoot     Root   `ssz-size:"32"`
	Body          BlockBody
}

type SignedBlock struct {
	Message   Block
	Signature Bytes32 `ssz-size:"32"`
}

// State is the main consensus state object.
type State struct {
	Config                   Config
	Slot                     Slot        `ssz-size:"8"`
	LatestBlockHeader        BlockHeader
	LatestJustified          Checkpoint
	LatestFinalized          Checkpoint
	HistoricalBlockHashes    []Root `ssz-max:"262144"`
	JustifiedSlots           []byte `ssz-max:"262144" ssz:"bitlist"`
	JustificationsRoots      []Root `ssz-max:"262144"`
	JustificationsValidators []byte `ssz-max:"1073741824" ssz:"bitlist"`
}
