//go:build spectests

package spectests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/blockprocessor"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

type fcFixture map[string]fcTest

// mockedProofSentinel opens every placeholder aggregation proof the upstream
// filler emits in its default (non-real-crypto) mode, where the recursive SNARK
// merge is skipped for vectors that do not test crypto. The cross-client contract
// is that a proof carrying this prefix must be accepted without cryptographic
// verification; any proof or signature lacking it is verified for real. Individual
// attestation signatures are always real, so this only ever short-circuits
// aggregated proofs. This must never leak into production verification: an attacker
// could otherwise prefix a forged proof to bypass the check.
var mockedProofSentinel = []byte("\x00MOCKED-AGGREGATION-PROOF\x00")

func carriesMockedProof(proof []byte) bool {
	return bytes.HasPrefix(proof, mockedProofSentinel)
}

type fcTest struct {
	Network     string   `json:"network"`
	LeanEnv     string   `json:"leanEnv"`
	AnchorState fcState  `json:"anchorState"`
	AnchorBlock fcBlock  `json:"anchorBlock"`
	Steps       []fcStep `json:"steps"`
}

type fcState struct {
	Config                   fcConfig        `json:"config"`
	Slot                     uint64          `json:"slot"`
	LatestBlockHeader        fcBlockHeader   `json:"latestBlockHeader"`
	LatestJustified          fcCheckpoint    `json:"latestJustified"`
	LatestFinalized          fcCheckpoint    `json:"latestFinalized"`
	HistoricalBlockHashes    fcDataList      `json:"historicalBlockHashes"`
	JustifiedSlots           fcDataList      `json:"justifiedSlots"`
	Validators               fcValidatorList `json:"validators"`
	JustificationsRoots      fcDataList      `json:"justificationsRoots"`
	JustificationsValidators fcDataList      `json:"justificationsValidators"`
}

type fcConfig struct {
	GenesisTime uint64 `json:"genesisTime"`
}

type fcBlockHeader struct {
	Slot          uint64 `json:"slot"`
	ProposerIndex uint64 `json:"proposerIndex"`
	ParentRoot    string `json:"parentRoot"`
	StateRoot     string `json:"stateRoot"`
	BodyRoot      string `json:"bodyRoot"`
}

type fcCheckpoint struct {
	Root string `json:"root"`
	Slot uint64 `json:"slot"`
}

type fcDataList struct {
	Data []json.RawMessage `json:"data"`
}

type fcValidator struct {
	AttestationPubkey string `json:"attestationPublicKey"`
	ProposalPubkey    string `json:"proposalPublicKey"`
	Pubkey            string `json:"pubkey"` // legacy fallback
	Index             uint64 `json:"index"`
}

type fcValidatorList struct {
	Data []fcValidator `json:"data"`
}

type fcBlock struct {
	Slot           uint64      `json:"slot"`
	ProposerIndex  uint64      `json:"proposerIndex"`
	ParentRoot     string      `json:"parentRoot"`
	StateRoot      string      `json:"stateRoot"`
	Body           fcBlockBody `json:"body"`
	BlockRootLabel string      `json:"blockRootLabel,omitempty"`
}

type fcBlockBody struct {
	Attestations fcDataList `json:"attestations"`
}

type fcStep struct {
	StepType    string               `json:"stepType"`
	Valid       bool                 `json:"valid"`
	Block       *fcBlock             `json:"block,omitempty"`
	Attestation *fcGossipAttestation `json:"attestation,omitempty"`
	Checks      *fcChecks            `json:"checks,omitempty"`
	Time        *uint64              `json:"time,omitempty"`
	Interval    *uint64              `json:"interval,omitempty"`
	HasProposal *bool                `json:"hasProposal,omitempty"`
	// TickToSlot reports whether the store clock advances to the block's slot
	// before import. Absent means the default (advance); false delivers the
	// block ahead of the store clock.
	TickToSlot *bool `json:"tickToSlot,omitempty"`
}

// fcGossipAttestation represents an individual gossip attestation step.
type fcGossipAttestation struct {
	ValidatorID uint64    `json:"validatorIndex"`
	Data        fcAttData `json:"data"`
	Signature   string    `json:"signature"`
	// Aggregated attestation fields (for gossipAggregatedAttestation steps).
	Proof *fcProof `json:"proof,omitempty"`
}

type fcProof struct {
	Participants fcDataList  `json:"participants"`
	Proof        fcProofData `json:"proof"`
}

type fcProofData struct {
	Data string `json:"data"`
}

type fcAttData struct {
	Slot   uint64       `json:"slot"`
	Head   fcCheckpoint `json:"head"`
	Target fcCheckpoint `json:"target"`
	Source fcCheckpoint `json:"source"`
}

type fcAggregatedAttestation struct {
	AggregationBits fcDataList `json:"aggregationBits"`
	Data            fcAttData  `json:"data"`
}

type fcChecks struct {
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
	AttestationChecks        []fcAttestationCheck `json:"attestationChecks,omitempty"`
	LexicographicHeadAmong   []string             `json:"lexicographicHeadAmong,omitempty"`
}

// fcAttestationCheck mirrors the spec's per-validator attestation-state
// expectations: each entry pins one validator's latest attestation slots
// in a specific store location ("known", "new", etc).
type fcAttestationCheck struct {
	Validator       uint64  `json:"validator"`
	AttestationSlot *uint64 `json:"attestationSlot,omitempty"`
	HeadSlot        *uint64 `json:"headSlot,omitempty"`
	SourceSlot      *uint64 `json:"sourceSlot,omitempty"`
	TargetSlot      *uint64 `json:"targetSlot,omitempty"`
	Location        string  `json:"location"`
}

// toState converts fixture anchor state to types.State.
func (fs *fcState) toState() *types.State {
	state := &types.State{
		Config: &types.ChainConfig{
			GenesisTime: fs.Config.GenesisTime,
		},
		Slot: fs.Slot,
		LatestBlockHeader: &types.BlockHeader{
			Slot:          fs.LatestBlockHeader.Slot,
			ProposerIndex: fs.LatestBlockHeader.ProposerIndex,
			ParentRoot:    parseHexRoot(fs.LatestBlockHeader.ParentRoot),
			StateRoot:     parseHexRoot(fs.LatestBlockHeader.StateRoot),
			BodyRoot:      parseHexRoot(fs.LatestBlockHeader.BodyRoot),
		},
		LatestJustified: &types.Checkpoint{
			Root: parseHexRoot(fs.LatestJustified.Root),
			Slot: fs.LatestJustified.Slot,
		},
		LatestFinalized: &types.Checkpoint{
			Root: parseHexRoot(fs.LatestFinalized.Root),
			Slot: fs.LatestFinalized.Slot,
		},
	}

	for _, v := range fs.Validators.Data {
		var attPk, propPk [types.PubkeySize]byte
		if v.AttestationPubkey != "" {
			attPk = parseHexPubkey(v.AttestationPubkey)
			propPk = parseHexPubkey(v.ProposalPubkey)
		} else {
			pk := parseHexPubkey(v.Pubkey)
			attPk = pk
			propPk = pk
		}
		state.Validators = append(state.Validators, &types.Validator{
			AttestationPubkey: attPk,
			ProposalPubkey:    propPk,
			Index:             v.Index,
		})
	}

	for _, raw := range fs.HistoricalBlockHashes.Data {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			panic(fmt.Sprintf("HistoricalBlockHashes: %v", err))
		}
		b := parseHexBytes(s)
		h := make([]byte, 32)
		copy(h, b)
		state.HistoricalBlockHashes = append(state.HistoricalBlockHashes, h)
	}

	state.JustifiedSlots = parseBoolBitlist(fs.JustifiedSlots.Data)

	for _, raw := range fs.JustificationsRoots.Data {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			panic(fmt.Sprintf("JustificationsRoots: %v", err))
		}
		b := parseHexBytes(s)
		h := make([]byte, 32)
		copy(h, b)
		state.JustificationsRoots = append(state.JustificationsRoots, h)
	}

	state.JustificationsValidators = parseBoolBitlist(fs.JustificationsValidators.Data)

	return state
}

// toBlock converts a fixture block to types.Block.
func (fb *fcBlock) toBlock() *types.Block {
	block := &types.Block{
		Slot:          fb.Slot,
		ProposerIndex: fb.ProposerIndex,
		ParentRoot:    parseHexRoot(fb.ParentRoot),
		StateRoot:     parseHexRoot(fb.StateRoot),
		Body: &types.BlockBody{
			Attestations: make([]*types.AggregatedAttestation, 0),
		},
	}

	for _, raw := range fb.Body.Attestations.Data {
		var ta fcAggregatedAttestation
		if err := json.Unmarshal(raw, &ta); err != nil {
			panic(fmt.Sprintf("attestation unmarshal: %v", err))
		}
		att := &types.AggregatedAttestation{
			AggregationBits: parseBoolBitlist(ta.AggregationBits.Data),
			Data: &types.AttestationData{
				Slot: ta.Data.Slot,
				Head: &types.Checkpoint{
					Root: parseHexRoot(ta.Data.Head.Root),
					Slot: ta.Data.Head.Slot,
				},
				Target: &types.Checkpoint{
					Root: parseHexRoot(ta.Data.Target.Root),
					Slot: ta.Data.Target.Slot,
				},
				Source: &types.Checkpoint{
					Root: parseHexRoot(ta.Data.Source.Root),
					Slot: ta.Data.Source.Slot,
				},
			},
		}
		block.Body.Attestations = append(block.Body.Attestations, att)
	}

	return block
}
func TestSpecForkChoice(t *testing.T) {
	logger.SetQuiet(true)
	defer logger.SetQuiet(false)

	fixtureDir := "../../leanSpec/fixtures/consensus/fork_choice"

	if _, err := os.Stat(fixtureDir); os.IsNotExist(err) {
		t.Skipf("fixtures not present at %s; skipping", fixtureDir)
	}

	var files []string
	err := filepath.Walk(fixtureDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".json" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking fixture dir %s: %v", fixtureDir, err)
	}

	if len(files) == 0 {
		t.Fatalf("no fixture files found in %s", fixtureDir)
	}

	for _, file := range files {
		file := file
		relPath, _ := filepath.Rel(fixtureDir, file)
		t.Run(relPath, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("reading %s: %v", file, err)
			}

			var fixture fcFixture
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatalf("unmarshalling %s: %v", file, err)
			}

			for testName, tt := range fixture {
				tt := tt
				t.Run(testName, func(t *testing.T) {
					runForkChoiceTest(t, &tt)
				})
			}
		})
	}
}

func runForkChoiceTest(t *testing.T, tt *fcTest) {
	t.Helper()

	// 1. Convert anchor state and block.
	anchorState := tt.AnchorState.toState()
	anchorBlock := tt.AnchorBlock.toBlock()

	// Compute anchor block root.
	anchorRoot, err := anchorBlock.HashTreeRoot()
	if err != nil {
		t.Fatalf("computing anchor block root: %v", err)
	}

	// 2. Initialize store with in-memory backend.
	backend := storage.NewInMemoryBackend()
	s := store.NewConsensusStore(backend)

	// Store config from anchor state.
	s.SetConfig(anchorState.Config)

	// Store anchor state + block header.
	anchorHeader := &types.BlockHeader{
		Slot:          anchorBlock.Slot,
		ProposerIndex: anchorBlock.ProposerIndex,
		ParentRoot:    anchorBlock.ParentRoot,
		StateRoot:     anchorBlock.StateRoot,
	}
	if anchorBlock.Body != nil {
		bodyRoot, _ := anchorBlock.Body.HashTreeRoot()
		anchorHeader.BodyRoot = bodyRoot
	}

	// Verify the anchor pair is self-consistent: the anchor block's state_root
	// must match hash_tree_root(anchor_state). Without this check, a malformed
	// fixture (or attacker-served (state, block) pair
	// in production) would be accepted after the runner silently rewrote
	// state.LatestBlockHeader.StateRoot to match. Direct compare; the spec
	// expects the input state to arrive with LatestBlockHeader.StateRoot
	// already in the canonical form that makes the equality hold.
	//
	// Empty-steps fixtures with a mismatched pair are negative tests: the
	// spec expected init to abort here. Treat the rejection as pass.
	computedStateRoot, err := anchorState.HashTreeRoot()
	if err != nil {
		t.Fatalf("hashing anchor state: %v", err)
	}
	if computedStateRoot != anchorBlock.StateRoot {
		if len(tt.Steps) == 0 {
			t.Logf("anchor init rejected (expected): block=%x state=%x",
				anchorBlock.StateRoot, computedStateRoot)
			return
		}
		t.Fatalf("anchor state-root mismatch: block=%x state=%x",
			anchorBlock.StateRoot, computedStateRoot)
	}

	s.InsertBlockHeader(anchorRoot, anchorHeader)
	s.InsertState(anchorRoot, anchorState)
	s.InsertLiveChainEntry(anchorBlock.Slot, anchorRoot, anchorBlock.ParentRoot)
	s.SetHead(anchorRoot)
	// Seed checkpoints from the anchor block itself, not from
	// anchorState.LatestJustified/LatestFinalized. The store treats the anchor
	// as the new genesis: justified/finalized point
	// at the anchor block, and any pre-anchor history embedded in the state's
	// checkpoints is intentionally ignored.
	s.SetLatestJustified(&types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot})
	s.SetLatestFinalized(&types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot})

	// Store anchor as signed block.
	anchorSigned := &types.SignedBlock{
		Block: anchorBlock,
		Proof: nil,
	}
	s.StorePendingBlock(anchorRoot, anchorSigned)

	// 3. Initialize fork choice with anchor.
	fc := forkchoice.New(anchorBlock.Slot, anchorRoot, anchorBlock.ParentRoot)

	// Label -> root map for resolving blockRootLabel references.
	labelRoots := make(map[string][32]byte)

	// 4. Process steps.
	for i, step := range tt.Steps {
		switch step.StepType {
		case "block":
			if step.Block == nil {
				t.Fatalf("step %d: block step without block data", i)
			}

			block := step.Block.toBlock()

			// Build signatures with participant bits from attestation aggregation_bits
			// so processBlockAttestations stores correct per-validator votes.
			signedBlock := &types.SignedBlock{
				Block: block,
				Proof: &types.MultiMessageAggregate{},
			}

			// Advance the store clock to this block's slot unless the fixture
			// delivers the block ahead of the clock (tickToSlot=false). The clock
			// gates attestation future-validation, so it must reach the slot
			// before subsequent attestations validate.
			if step.TickToSlot == nil || *step.TickToSlot {
				minTime := block.Slot * types.IntervalsPerSlot
				if s.Time() < minTime {
					s.SetTime(minTime)
				}
			}

			// Process block through store (no signature verification).
			if err := blockprocessor.OnBlockWithoutVerification(s, signedBlock); err != nil {
				if step.Valid {
					t.Fatalf("step %d: OnBlockWithoutVerification failed: %v", i, err)
				}
				continue
			}

			// Compute block root and register label.
			blockRoot, _ := block.HashTreeRoot()
			if step.Block.BlockRootLabel != "" {
				labelRoots[step.Block.BlockRootLabel] = blockRoot
			}

			// Register block in fork choice.
			fc.OnBlock(block.Slot, blockRoot, block.ParentRoot)

			// Seed the block's on-chain aggregated attestations into the known
			// pool with their participant sets so the votes carry fork-choice
			// weight. The spec test framework merges a block's aggregated proofs
			// into latest_known_aggregated_payloads when building it; block
			// import alone (data, empty proof set) leaves them weightless, which
			// would mis-resolve weight-driven reorgs. Only participants are read
			// during head computation, so a non-empty placeholder proof suffices.
			for _, att := range block.Body.Attestations {
				if att == nil || att.Data == nil || types.BitlistCount(att.AggregationBits) == 0 {
					continue
				}
				dataRoot, err := att.Data.HashTreeRoot()
				if err != nil {
					continue
				}
				s.KnownPayloads.Push(dataRoot, att.Data, &types.SingleMessageAggregate{
					Participants: att.AggregationBits,
					Proof:        []byte{0x01},
				})
			}

			// Update head: extract known attestations, feed to fork choice, compute head.
			attestations := s.ExtractLatestKnownAttestations()
			justifiedRoot := s.LatestJustified().Root

			for vid, data := range attestations {
				fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
			}

			simulateUpdateHead(s, fc, justifiedRoot)

			// Promote new payloads to known (so next updateHead sees them).
			s.PromoteNewToKnown()

			// Reflect the just-promoted votes into the fork-choice known tracker,
			// as the Engine's next updateHead re-derives known votes from the
			// promoted pool. Without this, a vote gossiped as "new" is promoted in
			// the payload pool but stays absent from the tracker the checks read.
			for vid, data := range s.ExtractLatestKnownAttestations() {
				fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
			}

			// Validate checks if present.
			if step.Checks != nil {
				validateChecks(t, i, step.Checks, s, fc, labelRoots)
			}

		case "attestation":
			if step.Attestation == nil {
				t.Fatalf("step %d: attestation step without data", i)
			}
			att := step.Attestation
			attData := &types.AttestationData{
				Slot:   att.Data.Slot,
				Head:   &types.Checkpoint{Root: parseHexRoot(att.Data.Head.Root), Slot: att.Data.Head.Slot},
				Target: &types.Checkpoint{Root: parseHexRoot(att.Data.Target.Root), Slot: att.Data.Target.Slot},
				Source: &types.Checkpoint{Root: parseHexRoot(att.Data.Source.Root), Slot: att.Data.Source.Slot},
			}

			// For valid steps, advance time so attestation passes the future check.
			// Invalid steps keep current time so the time-bound branch of
			// ValidateAttestationData fires as expected on fixtures designed to
			// exercise that rejection path.
			if step.Valid {
				minTime := attData.Slot * types.IntervalsPerSlot
				if s.Time() < minTime {
					s.SetTime(minTime)
				}
			}

			// Run data/bounds validation regardless of step.Valid so rejection
			// fixtures actually exercise the validator. Individual attestation
			// signatures are always real, so verification always runs here.
			dataRoot, _ := attData.HashTreeRoot()
			signature := parseHexBytes(att.Signature)
			var validationErr error
			if validationErr = attestation.ValidateAttestationData(s, attData); validationErr == nil && !carriesMockedProof(signature) {
				validationErr = attestation.VerifyGossipAttestation(s, att.ValidatorID, attData, dataRoot, signature)
			}
			if step.Valid && validationErr != nil {
				t.Fatalf("step %d: expected valid attestation, got error: %v", i, validationErr)
			}
			if !step.Valid && validationErr == nil {
				t.Fatalf("step %d: expected invalid attestation, got accepted", i)
			}
			if !step.Valid {
				// Validation correctly rejected — no state mutation, just
				// assert any checks the fixture carried then continue.
				if step.Checks != nil {
					validateChecks(t, i, step.Checks, s, fc, labelRoots)
				}
				continue
			}

			// Store in new payloads with dummy proof.
			participants := types.BitlistFromIndices([]uint64{att.ValidatorID})
			proof := &types.SingleMessageAggregate{
				Participants: participants,
				Proof:        nil,
			}
			s.NewPayloads.Push(dataRoot, attData, proof)

			// Record the raw per-validator signature, mirroring the spec's
			// attestation_signatures pool that the "signatures" check reads.
			var sig [types.SignatureSize]byte
			copy(sig[:], signature)
			s.AttestationSignatures.Insert(dataRoot, attData, att.ValidatorID, sig)

			// Feed vote to fork choice so attestation weight is reflected.
			fc.SetNewVote(att.ValidatorID, attData.Head.Root, attData.Slot, attData)

			// Gossip lands in the new pool only. The head keeps reflecting the
			// known pool until a slot-boundary tick promotes these votes, so
			// recompute from the known pool here without promoting.
			knownAtts := s.ExtractLatestKnownAttestations()
			justifiedRoot := s.LatestJustified().Root
			for vid, data := range knownAtts {
				fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
			}
			simulateUpdateHead(s, fc, justifiedRoot)

			if step.Checks != nil {
				validateChecks(t, i, step.Checks, s, fc, labelRoots)
			}

		case "gossipAggregatedAttestation":
			if step.Attestation == nil {
				t.Fatalf("step %d: gossipAggregatedAttestation step without data", i)
			}
			att := step.Attestation
			attData := &types.AttestationData{
				Slot:   att.Data.Slot,
				Head:   &types.Checkpoint{Root: parseHexRoot(att.Data.Head.Root), Slot: att.Data.Head.Slot},
				Target: &types.Checkpoint{Root: parseHexRoot(att.Data.Target.Root), Slot: att.Data.Target.Slot},
				Source: &types.Checkpoint{Root: parseHexRoot(att.Data.Source.Root), Slot: att.Data.Source.Slot},
			}

			// For valid steps, advance time so attestation passes the future check.
			// Invalid steps keep current time to exercise the time-bound branch.
			if step.Valid {
				minTime := attData.Slot * types.IntervalsPerSlot
				if s.Time() < minTime {
					s.SetTime(minTime)
				}
			}

			var participants []byte
			var proofData []byte
			if att.Proof != nil {
				participants = parseBoolBitlist(att.Proof.Participants.Data)
				proofData = parseHexBytes(att.Proof.Proof.Data)
			}

			// Symmetric with the individual-attestation case: data/bounds checks
			// always run; the aggregated-proof crypto check is skipped only when
			// the proof is a mocked placeholder. Real proofs — including the short
			// proofs the registry/empty-participant rejection vectors carry — fall
			// through to full verification.
			dataRoot, _ := attData.HashTreeRoot()
			var validationErr error
			if validationErr = attestation.ValidateAttestationData(s, attData); validationErr == nil && !carriesMockedProof(proofData) {
				validationErr = attestation.VerifyAggregatedGossipAttestation(s, attData, participants, proofData)
			}
			if step.Valid && validationErr != nil {
				t.Fatalf("step %d: expected valid aggregated attestation, got error: %v", i, validationErr)
			}
			if !step.Valid && validationErr == nil {
				t.Fatalf("step %d: expected invalid aggregated attestation, got accepted", i)
			}
			if !step.Valid {
				if step.Checks != nil {
					validateChecks(t, i, step.Checks, s, fc, labelRoots)
				}
				continue
			}

			proof := &types.SingleMessageAggregate{
				Participants: participants,
				Proof:        proofData,
			}
			s.NewPayloads.Push(dataRoot, attData, proof)

			// Feed per-validator votes to fork choice from participant bits.
			participantIDs := types.BitlistIndices(participants)
			for _, vid := range participantIDs {
				fc.SetNewVote(vid, attData.Head.Root, attData.Slot, attData)
			}

			// Gossip lands in the new pool only. The head keeps reflecting the
			// known pool until a slot-boundary tick promotes these votes, so
			// recompute from the known pool here without promoting.
			knownAtts := s.ExtractLatestKnownAttestations()
			justifiedRoot := s.LatestJustified().Root
			for vid, data := range knownAtts {
				fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
			}
			simulateUpdateHead(s, fc, justifiedRoot)

			if step.Checks != nil {
				validateChecks(t, i, step.Checks, s, fc, labelRoots)
			}

		case "tick":
			// step.Time is wall-clock seconds since the UNIX epoch; step.Interval
			// is a raw interval count. Convert seconds to intervals before storing
			// so subsequent assertions on store.time match the simulator's
			// checks.time field.
			switch {
			case step.Time != nil:
				genesisMs := s.Config().GenesisTime * 1000
				timestampMs := *step.Time * 1000
				if timestampMs < genesisMs {
					s.SetTime(0)
				} else {
					s.SetTime((timestampMs - genesisMs) / types.MillisecondsPerInterval)
				}
			case step.Interval != nil:
				s.SetTime(*step.Interval)
			default:
				t.Fatalf("step %d: tick step without time or interval", i)
			}

			// Mirror the interval responsibilities Engine.onTick drives in the
			// running node. Promotion of the new-vote pool into the known pool is
			// what flips the head onto freshly gossiped votes, and it only happens
			// at the slot boundary: interval 4 always, interval 0 when this slot is
			// ours to propose. The head is then recomputed at interval 0/4.
			interval := s.Time() % types.IntervalsPerSlot
			hasProposal := step.HasProposal != nil && *step.HasProposal
			if interval == 4 || (interval == 0 && hasProposal) {
				s.PromoteNewToKnown()
			}
			if interval == 0 || interval == 4 {
				knownAtts := s.ExtractLatestKnownAttestations()
				for vid, data := range knownAtts {
					fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
				}
				simulateUpdateHead(s, fc, s.LatestJustified().Root)
			}

		default:
			t.Fatalf("step %d: unknown step type %q", i, step.StepType)
		}
	}
}

func validateChecks(t *testing.T, stepIdx int, checks *fcChecks, s *store.ConsensusStore, fc *forkchoice.ForkChoice, labelRoots map[string][32]byte) {
	t.Helper()

	headRoot := s.Head()

	if checks.HeadSlot != nil {
		headHeader := s.GetBlockHeader(headRoot)
		if headHeader == nil {
			t.Fatalf("step %d check: head block header not found for root 0x%x", stepIdx, headRoot)
		}
		if headHeader.Slot != *checks.HeadSlot {
			t.Fatalf("step %d check: headSlot got %d, want %d", stepIdx, headHeader.Slot, *checks.HeadSlot)
		}
	}

	if checks.HeadRoot != nil {
		wantRoot := parseHexRoot(*checks.HeadRoot)
		if headRoot != wantRoot {
			t.Fatalf("step %d check: headRoot got 0x%x, want 0x%x", stepIdx, headRoot, wantRoot)
		}
	}

	if checks.HeadRootLabel != nil {
		if labelRoot, ok := labelRoots[*checks.HeadRootLabel]; ok {
			if headRoot != labelRoot {
				t.Fatalf("step %d check: headRootLabel %q got 0x%x, want 0x%x",
					stepIdx, *checks.HeadRootLabel, headRoot, labelRoot)
			}
		}
	}

	if checks.Time != nil {
		if got := s.Time(); got != *checks.Time {
			t.Fatalf("step %d check: time got %d, want %d", stepIdx, got, *checks.Time)
		}
	}

	lj := s.LatestJustified()
	if checks.LatestJustifiedSlot != nil && lj.Slot != *checks.LatestJustifiedSlot {
		t.Fatalf("step %d check: latestJustifiedSlot got %d, want %d",
			stepIdx, lj.Slot, *checks.LatestJustifiedSlot)
	}
	if checks.LatestJustifiedRoot != nil {
		want := parseHexRoot(*checks.LatestJustifiedRoot)
		if lj.Root != want {
			t.Fatalf("step %d check: latestJustifiedRoot got 0x%x, want 0x%x",
				stepIdx, lj.Root, want)
		}
	}
	if checks.LatestJustifiedRootLabel != nil {
		if labelRoot, ok := labelRoots[*checks.LatestJustifiedRootLabel]; ok && lj.Root != labelRoot {
			t.Fatalf("step %d check: latestJustifiedRootLabel %q got 0x%x, want 0x%x",
				stepIdx, *checks.LatestJustifiedRootLabel, lj.Root, labelRoot)
		}
	}

	lf := s.LatestFinalized()
	if checks.LatestFinalizedSlot != nil && lf.Slot != *checks.LatestFinalizedSlot {
		t.Fatalf("step %d check: latestFinalizedSlot got %d, want %d",
			stepIdx, lf.Slot, *checks.LatestFinalizedSlot)
	}
	if checks.LatestFinalizedRoot != nil {
		want := parseHexRoot(*checks.LatestFinalizedRoot)
		if lf.Root != want {
			t.Fatalf("step %d check: latestFinalizedRoot got 0x%x, want 0x%x",
				stepIdx, lf.Root, want)
		}
	}
	if checks.LatestFinalizedRootLabel != nil {
		if labelRoot, ok := labelRoots[*checks.LatestFinalizedRootLabel]; ok && lf.Root != labelRoot {
			t.Fatalf("step %d check: latestFinalizedRootLabel %q got 0x%x, want 0x%x",
				stepIdx, *checks.LatestFinalizedRootLabel, lf.Root, labelRoot)
		}
	}

	// Simulate Engine.updateSafeTarget() before reading SafeTarget. Engine
	// fires this at interval 3; the runner cannot model interval cadence
	// directly, so it runs on demand whenever a safeTarget* check is set.
	if checks.SafeTarget != nil || checks.SafeTargetSlot != nil || checks.SafeTargetRootLabel != nil {
		simulateUpdateSafeTarget(s, fc)
	}
	st := s.SafeTarget()
	if checks.SafeTarget != nil {
		want := parseHexRoot(*checks.SafeTarget)
		if st != want {
			t.Fatalf("step %d check: safeTarget got 0x%x, want 0x%x", stepIdx, st, want)
		}
	}
	if checks.SafeTargetRootLabel != nil {
		if labelRoot, ok := labelRoots[*checks.SafeTargetRootLabel]; ok && st != labelRoot {
			t.Fatalf("step %d check: safeTargetRootLabel %q got 0x%x, want 0x%x",
				stepIdx, *checks.SafeTargetRootLabel, st, labelRoot)
		}
	}
	if checks.SafeTargetSlot != nil {
		stHeader := s.GetBlockHeader(st)
		if stHeader == nil {
			t.Fatalf("step %d check: safe target header not found for root 0x%x", stepIdx, st)
		}
		if stHeader.Slot != *checks.SafeTargetSlot {
			t.Fatalf("step %d check: safeTargetSlot got %d, want %d",
				stepIdx, stHeader.Slot, *checks.SafeTargetSlot)
		}
	}

	if checks.AttestationTargetSlot != nil {
		target := attestation.GetAttestationTarget(s)
		if target.Slot != *checks.AttestationTargetSlot {
			t.Fatalf("step %d check: attestationTargetSlot got %d, want %d",
				stepIdx, target.Slot, *checks.AttestationTargetSlot)
		}
	}

	if len(checks.LexicographicHeadAmong) > 0 {
		found := false
		for _, label := range checks.LexicographicHeadAmong {
			if root, ok := labelRoots[label]; ok && root == headRoot {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("step %d check: head 0x%x not in lexicographicHeadAmong %v",
				stepIdx, headRoot, checks.LexicographicHeadAmong)
		}
	}

	for _, ac := range checks.AttestationChecks {
		validateAttestationCheck(t, stepIdx, s, fc, ac)
	}
}

func validateAttestationCheck(t *testing.T, stepIdx int, s *store.ConsensusStore, fc *forkchoice.ForkChoice, ac fcAttestationCheck) {
	t.Helper()
	var target *forkchoice.VoteTarget
	switch ac.Location {
	case "new", "known":
		tracker, ok := fc.VoteTracker(ac.Validator)
		if !ok {
			t.Fatalf("step %d: attestationCheck v=%d location=%q: no vote tracker", stepIdx, ac.Validator, ac.Location)
		}
		if ac.Location == "new" {
			target = tracker.LatestNew
		} else {
			target = tracker.LatestKnown
		}
	case "signatures":
		target = latestSignatureVote(s, ac.Validator)
	default:
		t.Fatalf("step %d: attestationCheck v=%d: unsupported location %q (want \"new\", \"known\" or \"signatures\")",
			stepIdx, ac.Validator, ac.Location)
	}
	if target == nil {
		t.Fatalf("step %d: attestationCheck v=%d %s: no attestation present in pool",
			stepIdx, ac.Validator, ac.Location)
	}
	if ac.AttestationSlot != nil && target.Slot != *ac.AttestationSlot {
		t.Errorf("step %d: attestationCheck v=%d %s: attestationSlot got %d, want %d",
			stepIdx, ac.Validator, ac.Location, target.Slot, *ac.AttestationSlot)
	}
	if target.Data == nil {
		if ac.HeadSlot != nil || ac.SourceSlot != nil || ac.TargetSlot != nil {
			t.Fatalf("step %d: attestationCheck v=%d %s: tracker has no AttestationData",
				stepIdx, ac.Validator, ac.Location)
		}
		return
	}
	if ac.HeadSlot != nil && target.Data.Head != nil && target.Data.Head.Slot != *ac.HeadSlot {
		t.Errorf("step %d: attestationCheck v=%d %s: headSlot got %d, want %d",
			stepIdx, ac.Validator, ac.Location, target.Data.Head.Slot, *ac.HeadSlot)
	}
	if ac.SourceSlot != nil && target.Data.Source != nil && target.Data.Source.Slot != *ac.SourceSlot {
		t.Errorf("step %d: attestationCheck v=%d %s: sourceSlot got %d, want %d",
			stepIdx, ac.Validator, ac.Location, target.Data.Source.Slot, *ac.SourceSlot)
	}
	if ac.TargetSlot != nil && target.Data.Target != nil && target.Data.Target.Slot != *ac.TargetSlot {
		t.Errorf("step %d: attestationCheck v=%d %s: targetSlot got %d, want %d",
			stepIdx, ac.Validator, ac.Location, target.Data.Target.Slot, *ac.TargetSlot)
	}
}

// latestSignatureVote mirrors the spec's "signatures" location: scan the raw
// attestation-signature pool and return the validator's highest-slot vote
// (first seen wins on equal slots, matching the fork-choice rule). Returns nil
// when the validator has no signature recorded.
func latestSignatureVote(s *store.ConsensusStore, validator uint64) *forkchoice.VoteTarget {
	var latest *types.AttestationData
	for _, entry := range s.AttestationSignatures.Snapshot() {
		for _, sig := range entry.Signatures {
			if sig.ValidatorID != validator {
				continue
			}
			if latest == nil || latest.Slot < entry.Data.Slot {
				latest = entry.Data
			}
		}
	}
	if latest == nil {
		return nil
	}
	return &forkchoice.VoteTarget{Slot: latest.Slot, Data: latest}
}

// simulateUpdateHead recomputes the head and re-anchors finalization to the head's
// chain, mirroring the spec's update_head. Finalization is derived from the canonical
// head rather than advanced during block import, matching the production node.
func simulateUpdateHead(s *store.ConsensusStore, fc *forkchoice.ForkChoice, justifiedRoot [32]byte) {
	newHead := fc.UpdateHead(justifiedRoot)
	s.SetHead(newHead)
	// Track the canonical head's finalized checkpoint unconditionally, mirroring
	// the spec: a higher-finalized fork that loses head selection must not latch.
	if derived := store.DeriveFinalizedFromHead(s, newHead); derived != nil {
		s.SetLatestFinalized(derived)
	}
}

func simulateUpdateSafeTarget(s *store.ConsensusStore, fc *forkchoice.ForkChoice) {
	headState := s.GetState(s.Head())
	if headState == nil {
		return
	}
	justifiedRoot := s.LatestJustified().Root
	numValidators := uint64(len(headState.Validators))
	safeTarget := fc.UpdateSafeTarget(justifiedRoot, numValidators)
	s.SetSafeTarget(safeTarget)
}
