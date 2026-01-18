package types

//go:generate sszgen --path=. --objs=Checkpoint,Config,Validator,AttestationData,Attestation,AggregatedAttestation,BlockHeader,BlockBody,Block,State

type Slot uint64
type ValidatorIndex uint64
type Epoch uint64
type Root [32]byte

type Bytes4 [4]byte
type Bytes20 [20]byte
type Bytes32 = Root
type Bytes48 [48]byte
type Bytes52 [52]byte
type Bytes96 [96]byte

const (
	SecondsPerSlot         uint64 = 4
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

type Checkpoint struct {
	Root Root `ssz-size:"32"`
	Slot Slot `ssz-size:"8"`
}

// Config holds chain configuration parameters.
type Config struct {
	GenesisTime uint64 `ssz-size:"8"`
}

// Validator represents a validator's metadata.
type Validator struct {
	Pubkey Bytes52 `ssz-size:"52"`
	Index  uint64  `ssz-size:"8"`
}

// AttestationData describes a validator's view of the chain.
type AttestationData struct {
	Slot   Slot `ssz-size:"8"`
	Head   Checkpoint
	Target Checkpoint
	Source Checkpoint
}

// Attestation is a single validator's attestation.
type Attestation struct {
	ValidatorID uint64 `ssz-size:"8"`
	Data        AttestationData
}

// AggregatedAttestation combines multiple validators' attestations.
type AggregatedAttestation struct {
	AggregationBits []byte `ssz-max:"4096" ssz:"bitlist"` // ValidatorRegistryLimit
	Data            AttestationData
}

// BlockHeader summarizes a block without the body.
type BlockHeader struct {
	Slot          Slot   `ssz-size:"8"`
	ProposerIndex uint64 `ssz-size:"8"`
	ParentRoot    Root   `ssz-size:"32"`
	StateRoot     Root   `ssz-size:"32"`
	BodyRoot      Root   `ssz-size:"32"`
}

// BlockBody contains the block's payload.
type BlockBody struct {
	Attestations []AggregatedAttestation `ssz-max:"4096"` // ValidatorRegistryLimit
}

// Block is a complete block including header fields and body.
type Block struct {
	Slot          Slot   `ssz-size:"8"`
	ProposerIndex uint64 `ssz-size:"8"`
	ParentRoot    Root   `ssz-size:"32"`
	StateRoot     Root   `ssz-size:"32"`
	Body          BlockBody
}

// State is the beacon state.
type State struct {
	Config             Config
	Slot               Slot `ssz-size:"8"`
	LatestBlockHeader  BlockHeader
	LatestJustified    Checkpoint
	LatestFinalized    Checkpoint
	HistoricalRoots    []Root      `ssz-max:"262144"`                    // HistoricalRootsLimit
	JustifiedSlots     []byte      `ssz-max:"262144" ssz:"bitlist"`      // HistoricalRootsLimit
	Validators         []Validator `ssz-max:"4096"`                      // ValidatorRegistryLimit
	JustificationRoots []Root      `ssz-max:"262144"`                    // HistoricalRootsLimit
	JustificationVotes []byte      `ssz-max:"1073741824" ssz:"bitlist"` // 2^30 (262144 × 4096)
}
