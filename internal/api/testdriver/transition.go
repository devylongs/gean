package testdriver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/geanlabs/gean/internal/specfixtures"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/types"
)

func StateTransitionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var fixture specfixtures.StateTransitionFixture
		if err := json.NewDecoder(r.Body).Decode(&fixture); err != nil {
			http.Error(w, fmt.Sprintf("decode body: %v", err), http.StatusBadRequest)
			return
		}

		state, err := fixture.Pre.ToState()
		if err != nil {
			writeStateTransitionFailure(w, fmt.Sprintf("pre.toState: %v", err))
			return
		}

		if len(fixture.Blocks) == 0 {
			if fixture.ExpectedException() != "" {
				if err := statetransition.ProcessSlots(state, state.Slot); err != nil {
					writeStateTransitionFailure(w, err.Error())
					return
				}
			}
			writeStateTransitionSuccess(w, state)
			return
		}

		for i, tb := range fixture.Blocks {
			block, err := tb.ToBlock()
			if err != nil {
				writeStateTransitionFailure(w, fmt.Sprintf("blocks[%d]: %v", i, err))
				return
			}
			if err := statetransition.StateTransition(state, block); err != nil {
				writeStateTransitionFailure(w, fmt.Sprintf("blocks[%d]: %v", i, err))
				return
			}
		}

		writeStateTransitionSuccess(w, state)
	}
}

func writeStateTransitionSuccess(w http.ResponseWriter, state *types.State) {
	resp := stateTransitionResponse{
		Succeeded: true,
		Error:     nil,
		Post: &stateTransitionPost{
			Slot:                       state.Slot,
			LatestBlockHeaderSlot:      state.LatestBlockHeader.Slot,
			LatestBlockHeaderStateRoot: fmt.Sprintf("0x%x", state.LatestBlockHeader.StateRoot),
			HistoricalBlockHashesCount: uint64(len(state.HistoricalBlockHashes)),
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeStateTransitionFailure(w http.ResponseWriter, msg string) {
	resp := stateTransitionResponse{
		Succeeded: false,
		Error:     &msg,
		Post:      nil,
	}
	writeJSON(w, http.StatusOK, resp)
}
