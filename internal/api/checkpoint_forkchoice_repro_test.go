package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/geanlabs/gean/internal/checkpoint"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// TestCheckpointSyncReachesNonGenesisFinalizedForkChoice reproduces the hive
// rpc-compat "forkchoice keeps nodes at or beyond finalized slot" test locally:
// a client given a valid checkpoint source (no peers) must checkpoint-sync and
// report finalized.slot > 0 on /lean/v0/fork_choice. It exercises the real
// checkpoint fetch, the store seeding bootstrap performs, and the real handler.
func TestCheckpointSyncReachesNonGenesisFinalizedForkChoice(t *testing.T) {
	const genesisTime = 1000
	const finalizedSlot = 100

	state, signed := makeReproAnchorPair(t, finalizedSlot, genesisTime, 3)

	// Stand up a mock finalized source at the paths the client derives.
	mux := http.NewServeMux()
	mux.HandleFunc(checkpoint.StatesFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(mustSSZ(t, state))
	})
	mux.HandleFunc(checkpoint.BlocksFinalizedPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(mustSSZ(t, signed))
	})
	src := httptest.NewServer(mux)
	defer src.Close()

	// Real checkpoint sync against the source.
	anchorState, anchorBlock, err := checkpoint.FetchCheckpointAnchor(src.URL+checkpoint.StatesFinalizedPath, genesisTime, state.Validators)
	if err != nil {
		t.Fatalf("checkpoint sync failed against a valid source: %v", err)
	}

	// Seed the store exactly as bootstrap does, then serve fork_choice.
	s := store.NewConsensusStore(storage.NewInMemoryBackend())
	seedStoreFromAnchor(t, s, anchorState, anchorBlock)
	fc := forkchoice.New(anchorState.Slot, s.Head(), anchorState.LatestBlockHeader.ParentRoot)

	rec := httptest.NewRecorder()
	ForkChoiceHandler(s, fc)(rec, httptest.NewRequest(http.MethodGet, "/lean/v0/fork_choice", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("fork_choice status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Finalized struct {
			Slot uint64 `json:"slot"`
		} `json:"finalized"`
		Justified struct {
			Slot uint64 `json:"slot"`
		} `json:"justified"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode fork_choice: %v", err)
	}
	if resp.Finalized.Slot == 0 {
		t.Fatalf("fork_choice reports finalized.slot=0 after checkpoint sync; the poll would never see a non-genesis boundary")
	}
	if resp.Finalized.Slot != finalizedSlot {
		t.Fatalf("finalized.slot=%d, want %d", resp.Finalized.Slot, finalizedSlot)
	}
}

// seedStoreFromAnchor mirrors cmd/gean initStoreFromState + bootstrapFromCheckpoint.
func seedStoreFromAnchor(t *testing.T, s *store.ConsensusStore, state *types.State, signed *types.SignedBlock) {
	t.Helper()
	header := state.LatestBlockHeader
	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("state htr: %v", err)
	}
	if header.StateRoot == types.ZeroRoot {
		header.StateRoot = stateRoot
	}
	blockRoot, err := header.HashTreeRoot()
	if err != nil {
		t.Fatalf("header htr: %v", err)
	}
	anchor := &types.Checkpoint{Root: blockRoot, Slot: header.Slot}
	for _, put := range []func() error{
		func() error { return s.PutConfig(state.Config) },
		func() error { return s.PutHead(blockRoot) },
		func() error { return s.PutSafeTarget(blockRoot) },
		func() error { return s.PutLatestJustified(anchor) },
		func() error { return s.PutLatestFinalized(anchor) },
		func() error { return s.PutBlockHeader(blockRoot, header) },
		func() error { return s.PutState(blockRoot, state) },
		func() error { return s.StorePendingBlock(blockRoot, signed) },
	} {
		if err := put(); err != nil {
			t.Fatalf("seed store: %v", err)
		}
	}
}

func makeReproAnchorPair(t *testing.T, slot, genesisTime uint64, numValidators int) (*types.State, *types.SignedBlock) {
	t.Helper()
	validators := make([]*types.Validator, numValidators)
	for i := range validators {
		validators[i] = &types.Validator{
			AttestationPubkey: [types.PubkeySize]byte{byte(i + 1)},
			ProposalPubkey:    [types.PubkeySize]byte{byte(i + 11)},
			Index:             uint64(i),
		}
	}
	state := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: genesisTime},
		Slot:                     slot,
		LatestBlockHeader:        &types.BlockHeader{Slot: slot},
		LatestJustified:          &types.Checkpoint{Slot: slot - 2},
		LatestFinalized:          &types.Checkpoint{Slot: slot - 5},
		Validators:               validators,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
	body := &types.BlockBody{}
	bodyRoot, err := body.HashTreeRoot()
	if err != nil {
		t.Fatalf("body htr: %v", err)
	}
	state.LatestBlockHeader.BodyRoot = bodyRoot
	state.LatestBlockHeader.StateRoot = types.ZeroRoot
	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		t.Fatalf("state htr: %v", err)
	}
	signed := &types.SignedBlock{
		Block: &types.Block{
			Slot:          state.Slot,
			ProposerIndex: state.LatestBlockHeader.ProposerIndex,
			ParentRoot:    state.LatestBlockHeader.ParentRoot,
			StateRoot:     stateRoot,
			Body:          body,
		},
		Proof: &types.MultiMessageAggregate{},
	}
	return state, signed
}

func mustSSZ(t *testing.T, v interface{ MarshalSSZ() ([]byte, error) }) []byte {
	t.Helper()
	b, err := v.MarshalSSZ()
	if err != nil {
		t.Fatalf("marshal ssz: %v", err)
	}
	return b
}
