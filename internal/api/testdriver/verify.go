package testdriver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/geanlabs/gean/internal/blockprocessor"
	"github.com/geanlabs/gean/internal/specfixtures"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func VerifySignaturesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var fixture specfixtures.VerifySignaturesFixture
		if err := json.NewDecoder(r.Body).Decode(&fixture); err != nil {
			http.Error(w, fmt.Sprintf("decode body: %v", err), http.StatusBadRequest)
			return
		}

		anchorState, err := fixture.AnchorState.ToState()
		if err != nil {
			writeVerifyFailure(w, fmt.Sprintf("anchorState: %v", err))
			return
		}
		stateRoot, err := anchorState.HashTreeRoot()
		if err != nil {
			writeVerifyFailure(w, fmt.Sprintf("anchorState.hashTreeRoot: %v", err))
			return
		}
		header := anchorState.LatestBlockHeader
		if header == nil {
			writeVerifyFailure(w, "anchorState.latestBlockHeader missing")
			return
		}
		if header.StateRoot == types.ZeroRoot {
			header.StateRoot = stateRoot
		}
		anchorRoot, err := header.HashTreeRoot()
		if err != nil {
			writeVerifyFailure(w, fmt.Sprintf("anchorHeader.hashTreeRoot: %v", err))
			return
		}

		consensusStore := store.NewConsensusStore(storage.NewInMemoryBackend())
		consensusStore.SetConfig(anchorState.Config)
		consensusStore.InsertBlockHeader(anchorRoot, header)
		consensusStore.InsertState(anchorRoot, anchorState)
		consensusStore.InsertLiveChainEntry(header.Slot, anchorRoot, header.ParentRoot)
		consensusStore.SetHead(anchorRoot)
		consensusStore.SetLatestJustified(&types.Checkpoint{Root: anchorRoot, Slot: header.Slot})
		consensusStore.SetLatestFinalized(&types.Checkpoint{Root: anchorRoot, Slot: header.Slot})

		signedBlock, err := fixture.SignedBlock.ToSignedBlock()
		if err != nil {
			writeVerifyFailure(w, err.Error())
			return
		}

		if err := blockprocessor.OnBlock(consensusStore, signedBlock); err != nil {
			writeVerifyFailure(w, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, verifySignaturesResponse{Succeeded: true, Error: nil})
	}
}

func writeVerifyFailure(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusOK, verifySignaturesResponse{Succeeded: false, Error: &msg})
}
