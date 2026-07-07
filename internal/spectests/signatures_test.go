//go:build spectests

package spectests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/geanlabs/gean/internal/blockprocessor"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// Fixture types for signature verification tests.

type sigFixture map[string]sigTest

type sigTest struct {
	Network         string   `json:"network"`
	LeanEnv         string   `json:"leanEnv"`
	AnchorState     sigState `json:"anchorState"`
	SignedBlock     sigSBA   `json:"signedBlock"`
	RejectionReason *string  `json:"rejectionReason"`
}

type sigState struct {
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

type sigSBA struct {
	Block fcBlock  `json:"block"`
	Proof sigProof `json:"proof"`
}

// MultiMessageAggregate serializes as {"proof": {"data": "0x..."}} — its proof
// field is a ByteList nested one level deeper than a bare ByteList.
type sigProof struct {
	Proof fcProofData `json:"proof"`
}

// toState converts fixture anchor state to types.State.
func (fs *sigState) toState() *types.State {
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

// toSignedBlock converts fixture signed block to types.SignedBlock.
func (sba *sigSBA) toSignedBlock() *types.SignedBlock {
	return &types.SignedBlock{
		Block: sba.Block.toBlock(),
		Proof: &types.MultiMessageAggregate{Proof: parseHexBytes(sba.Proof.Proof.Data)},
	}
}

// Test runner.

func TestSpecSignatures(t *testing.T) {
	logger.SetQuiet(true)
	defer logger.SetQuiet(false)

	fixtureDir := "../../leanSpec/fixtures/consensus/verify_signatures"

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
		t.Skipf("no fixture files found in %s; skipping", fixtureDir)
	}

	for _, file := range files {
		file := file
		relPath, _ := filepath.Rel(fixtureDir, file)
		t.Run(relPath, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("reading %s: %v", file, err)
			}

			var fixture sigFixture
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatalf("unmarshalling %s: %v", file, err)
			}

			for testName, tt := range fixture {
				tt := tt
				t.Run(testName, func(t *testing.T) {
					runSignatureTest(t, &tt)
				})
			}
		})
	}
}

func runSignatureTest(t *testing.T, tt *sigTest) {
	t.Helper()

	// 1. Convert anchor state.
	anchorState := tt.AnchorState.toState()

	// 2. Fill state root in header if zero (genesis case), then compute anchor block root.
	// Matches initStoreFromState in main.go.
	stateRoot, _ := anchorState.HashTreeRoot()
	header := anchorState.LatestBlockHeader
	if header.StateRoot == types.ZeroRoot {
		header.StateRoot = stateRoot
	}
	anchorRoot, err := header.HashTreeRoot()
	if err != nil {
		t.Fatalf("computing anchor block root: %v", err)
	}

	// 3. Initialize store with in-memory backend.
	backend := storage.NewInMemoryBackend()
	s := store.NewConsensusStore(backend)

	s.SetConfig(anchorState.Config)
	s.InsertBlockHeader(anchorRoot, header)
	s.InsertState(anchorRoot, anchorState)
	s.InsertLiveChainEntry(header.Slot, anchorRoot, header.ParentRoot)
	s.SetHead(anchorRoot)
	s.SetLatestJustified(&types.Checkpoint{Root: anchorRoot, Slot: header.Slot})
	s.SetLatestFinalized(&types.Checkpoint{Root: anchorRoot, Slot: header.Slot})

	signedBlock := tt.SignedBlock.toSignedBlock()

	// Place the store clock at the block's slot, as the live node does before
	// importing an on-time block; otherwise the future-horizon guard would reject a
	// block whose slot leads the default-zero clock.
	if signedBlock != nil && signedBlock.Block != nil {
		s.SetTime(signedBlock.Block.Slot * types.IntervalsPerSlot)
	}

	// 5. Call OnBlock WITH signature verification.
	err = blockprocessor.OnBlock(s, signedBlock)

	// 6. Check result against expectation.
	expectFailure := tt.RejectionReason != nil

	if expectFailure {
		if err == nil {
			t.Fatalf("expected failure (%q) but OnBlock succeeded", *tt.RejectionReason)
		}
	} else {
		if err != nil {
			t.Fatalf("expected success but OnBlock failed: %v", err)
		}
	}
}
