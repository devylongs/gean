package specfixtures

import "encoding/json"

type StateTransitionFixture struct {
	Network                string         `json:"network"`
	LeanEnv                string         `json:"leanEnv"`
	Pre                    TestState      `json:"pre"`
	Blocks                 []TestBlock    `json:"blocks"`
	Post                   *TestPostState `json:"post"`
	ExpectException        string         `json:"expectException"`
	ExpectExceptionMessage string         `json:"expectExceptionMessage"`
}

type TestState struct {
	Config                   TestConfig        `json:"config"`
	Slot                     uint64            `json:"slot"`
	LatestBlockHeader        TestBlockHeader   `json:"latestBlockHeader"`
	LatestJustified          TestCheckpoint    `json:"latestJustified"`
	LatestFinalized          TestCheckpoint    `json:"latestFinalized"`
	HistoricalBlockHashes    TestDataList      `json:"historicalBlockHashes"`
	JustifiedSlots           TestDataList      `json:"justifiedSlots"`
	Validators               TestValidatorList `json:"validators"`
	JustificationsRoots      TestDataList      `json:"justificationsRoots"`
	JustificationsValidators TestDataList      `json:"justificationsValidators"`
}

type TestConfig struct {
	GenesisTime uint64 `json:"genesisTime"`
}

type TestBlockHeader struct {
	Slot          uint64 `json:"slot"`
	ProposerIndex uint64 `json:"proposerIndex"`
	ParentRoot    string `json:"parentRoot"`
	StateRoot     string `json:"stateRoot"`
	BodyRoot      string `json:"bodyRoot"`
}

type TestCheckpoint struct {
	Root string `json:"root"`
	Slot uint64 `json:"slot"`
}

type TestBlock struct {
	Slot           uint64        `json:"slot"`
	ProposerIndex  uint64        `json:"proposerIndex"`
	ParentRoot     string        `json:"parentRoot"`
	StateRoot      string        `json:"stateRoot"`
	Body           TestBlockBody `json:"body"`
	BlockRootLabel string        `json:"blockRootLabel,omitempty"`
}

type TestBlockBody struct {
	Attestations TestDataList `json:"attestations"`
}

type TestDataList struct {
	Data []json.RawMessage `json:"data"`
}

type TestValidator struct {
	AttestationPubkey string `json:"attestationPublicKey"`
	ProposalPubkey    string `json:"proposalPublicKey"`
	Pubkey            string `json:"pubkey"`
	Index             uint64 `json:"index"`
}

type TestValidatorList struct {
	Data []TestValidator `json:"data"`
}

type TestAggregatedAttestation struct {
	AggregationBits TestDataList `json:"aggregationBits"`
	Data            TestAttData  `json:"data"`
}

type TestAttData struct {
	Slot   uint64         `json:"slot"`
	Head   TestCheckpoint `json:"head"`
	Target TestCheckpoint `json:"target"`
	Source TestCheckpoint `json:"source"`
}

type TestPostState struct {
	Slot                           *uint64       `json:"slot"`
	LatestBlockHeaderSlot          *uint64       `json:"latestBlockHeaderSlot"`
	LatestBlockHeaderStateRoot     *string       `json:"latestBlockHeaderStateRoot"`
	LatestBlockHeaderProposerIndex *uint64       `json:"latestBlockHeaderProposerIndex"`
	LatestBlockHeaderParentRoot    *string       `json:"latestBlockHeaderParentRoot"`
	LatestBlockHeaderBodyRoot      *string       `json:"latestBlockHeaderBodyRoot"`
	LatestJustifiedSlot            *uint64       `json:"latestJustifiedSlot"`
	LatestJustifiedRoot            *string       `json:"latestJustifiedRoot"`
	LatestFinalizedSlot            *uint64       `json:"latestFinalizedSlot"`
	LatestFinalizedRoot            *string       `json:"latestFinalizedRoot"`
	HistoricalBlockHashesCount     *uint64       `json:"historicalBlockHashesCount"`
	ValidatorCount                 *uint64       `json:"validatorCount"`
	ConfigGenesisTime              *uint64       `json:"configGenesisTime"`
	HistoricalBlockHashes          *TestDataList `json:"historicalBlockHashes"`
	JustifiedSlots                 *TestDataList `json:"justifiedSlots"`
	JustificationsRoots            *TestDataList `json:"justificationsRoots"`
	JustificationsValidators       *TestDataList `json:"justificationsValidators"`
}

type ForkChoiceInit struct {
	AnchorState TestState `json:"anchorState"`
	AnchorBlock TestBlock `json:"anchorBlock"`
	GenesisTime *uint64   `json:"genesisTime"`
}

type ForkChoiceStep struct {
	StepType    string               `json:"stepType"`
	Valid       bool                 `json:"valid"`
	Block       *TestBlock           `json:"block,omitempty"`
	Attestation *FCGossipAttestation `json:"attestation,omitempty"`
	Checks      *FCChecks            `json:"checks,omitempty"`
	Time        *uint64              `json:"time,omitempty"`
	Interval    *uint64              `json:"interval,omitempty"`
	HasProposal *bool                `json:"hasProposal,omitempty"`
}

type FCGossipAttestation struct {
	ValidatorID uint64      `json:"validatorIndex"`
	Data        TestAttData `json:"data"`
	Signature   string      `json:"signature"`
	Proof       *FCProof    `json:"proof,omitempty"`
}

type FCProof struct {
	Participants TestDataList `json:"participants"`
	Proof        FCProofData  `json:"proof"`
}

type FCProofData struct {
	Data string `json:"data"`
}

type FCChecks struct {
	Time                     *uint64              `json:"time,omitempty"`
	HeadSlot                 *uint64              `json:"headSlot,omitempty"`
	HeadRoot                 *string              `json:"headRoot,omitempty"`
	HeadRootLabel            *string              `json:"headRootLabel,omitempty"`
	LatestJustifiedSlot      *uint64              `json:"latestJustifiedSlot,omitempty"`
	LatestJustifiedRoot      *string              `json:"latestJustifiedRoot,omitempty"`
	LatestJustifiedRootLabel *string              `json:"latestJustifiedRootLabel,omitempty"`
	LatestFinalizedSlot      *uint64              `json:"latestFinalizedSlot,omitempty"`
	LatestFinalizedRoot      *string              `json:"latestFinalizedRoot,omitempty"`
	LatestFinalizedRootLabel *string              `json:"latestFinalizedRootLabel,omitempty"`
	SafeTarget               *string              `json:"safeTarget,omitempty"`
	SafeTargetSlot           *uint64              `json:"safeTargetSlot,omitempty"`
	SafeTargetRootLabel      *string              `json:"safeTargetRootLabel,omitempty"`
	AttestationTargetSlot    *uint64              `json:"attestationTargetSlot,omitempty"`
	AttestationChecks        []FCAttestationCheck `json:"attestationChecks,omitempty"`
	LexicographicHeadAmong   []string             `json:"lexicographicHeadAmong,omitempty"`
}

type FCAttestationCheck struct {
	Validator       uint64  `json:"validator"`
	AttestationSlot *uint64 `json:"attestationSlot,omitempty"`
	HeadSlot        *uint64 `json:"headSlot,omitempty"`
	SourceSlot      *uint64 `json:"sourceSlot,omitempty"`
	TargetSlot      *uint64 `json:"targetSlot,omitempty"`
	Location        string  `json:"location"`
}

type VerifySignaturesFixture struct {
	Network         string             `json:"network"`
	LeanEnv         string             `json:"leanEnv"`
	AnchorState     TestState          `json:"anchorState"`
	SignedBlock     FixtureSignedBlock `json:"signedBlock"`
	RejectionReason *string            `json:"rejectionReason"`
}

type FixtureSignedBlock struct {
	Block TestBlock   `json:"block"`
	Proof FCProofData `json:"proof"`
}
