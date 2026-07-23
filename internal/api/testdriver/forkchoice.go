package testdriver

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/specfixtures"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func (sess *Session) ForkChoiceInitHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req specfixtures.ForkChoiceInit
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("decode body: %v", err), http.StatusBadRequest)
			return
		}

		anchorState, err := req.AnchorState.ToState()
		if err != nil {
			writeInitFailure(w, fmt.Sprintf("anchorState: %v", err))
			return
		}
		if req.GenesisTime != nil {
			anchorState.Config.GenesisTime = *req.GenesisTime
		}

		anchorBlock, err := req.AnchorBlock.ToBlock()
		if err != nil {
			writeInitFailure(w, fmt.Sprintf("anchorBlock: %v", err))
			return
		}
		anchorRoot, err := anchorBlock.HashTreeRoot()
		if err != nil {
			writeInitFailure(w, fmt.Sprintf("anchorBlock.hashTreeRoot: %v", err))
			return
		}

		computedStateRoot, err := anchorState.HashTreeRoot()
		if err != nil {
			writeInitFailure(w, fmt.Sprintf("anchorState.hashTreeRoot: %v", err))
			return
		}
		if computedStateRoot != anchorBlock.StateRoot {
			writeInitFailure(w, fmt.Sprintf("anchor state-root mismatch: block=0x%x state=0x%x",
				anchorBlock.StateRoot, computedStateRoot))
			return
		}

		anchorHeader := &types.BlockHeader{
			Slot:          anchorBlock.Slot,
			ProposerIndex: anchorBlock.ProposerIndex,
			ParentRoot:    anchorBlock.ParentRoot,
			StateRoot:     anchorBlock.StateRoot,
		}
		if anchorBlock.Body != nil {
			bodyRoot, err := anchorBlock.Body.HashTreeRoot()
			if err != nil {
				writeInitFailure(w, fmt.Sprintf("anchorBlock.body.hashTreeRoot: %v", err))
				return
			}
			anchorHeader.BodyRoot = bodyRoot
		}

		consensusStore := store.NewConsensusStore(storage.NewInMemoryBackend())
		if err := consensusStore.PutConfig(anchorState.Config); err != nil {
			writeInitFailure(w, fmt.Sprintf("store config: %v", err))
			return
		}
		if err := consensusStore.PutBlockHeader(anchorRoot, anchorHeader); err != nil {
			writeInitFailure(w, fmt.Sprintf("store block header: %v", err))
			return
		}
		if err := consensusStore.PutState(anchorRoot, anchorState); err != nil {
			writeInitFailure(w, fmt.Sprintf("store state: %v", err))
			return
		}
		if err := consensusStore.PutLiveChainEntry(anchorBlock.Slot, anchorRoot, anchorBlock.ParentRoot); err != nil {
			writeInitFailure(w, fmt.Sprintf("store live chain entry: %v", err))
			return
		}
		if err := consensusStore.PutHead(anchorRoot); err != nil {
			writeInitFailure(w, fmt.Sprintf("store head: %v", err))
			return
		}
		anchorCp := &types.Checkpoint{Root: anchorRoot, Slot: anchorBlock.Slot}
		if err := consensusStore.PutLatestJustified(anchorCp); err != nil {
			writeInitFailure(w, fmt.Sprintf("store latest justified: %v", err))
			return
		}
		if err := consensusStore.PutLatestFinalized(anchorCp); err != nil {
			writeInitFailure(w, fmt.Sprintf("store latest finalized: %v", err))
			return
		}
		if err := consensusStore.StorePendingBlock(anchorRoot, &types.SignedBlock{Block: anchorBlock}); err != nil {
			writeInitFailure(w, fmt.Sprintf("store anchor block: %v", err))
			return
		}

		fc := forkchoice.New(anchorBlock.Slot, anchorRoot, anchorBlock.ParentRoot)

		sess.mu.Lock()
		sess.store = consensusStore
		sess.fc = fc
		sess.labelRoots = make(map[string][32]byte)
		sess.mu.Unlock()

		w.WriteHeader(http.StatusNoContent)
	}
}

func stepChecksSafeTarget(step *specfixtures.ForkChoiceStep) bool {
	c := step.Checks
	return c != nil && (c.SafeTarget != nil || c.SafeTargetSlot != nil || c.SafeTargetRootLabel != nil)
}

func writeInitFailure(w http.ResponseWriter, msg string) {
	resp := driverStepResponse{Accepted: false, Error: &msg, Snapshot: driverSnapshot{}}
	writeJSON(w, http.StatusBadRequest, resp)
}

func (sess *Session) ForkChoiceStepHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var step specfixtures.ForkChoiceStep
		if err := json.NewDecoder(r.Body).Decode(&step); err != nil {
			http.Error(w, fmt.Sprintf("decode body: %v", err), http.StatusBadRequest)
			return
		}

		sess.mu.Lock()
		defer sess.mu.Unlock()

		if sess.store == nil || sess.fc == nil {
			msg := "fork_choice/step called before fork_choice/init"
			resp := driverStepResponse{Accepted: false, Error: &msg, Snapshot: driverSnapshot{}}
			writeJSON(w, http.StatusOK, resp)
			return
		}

		var stepErr error
		switch step.StepType {
		case "tick":
			stepErr = sess.applyTick(&step)
		case "block":
			stepErr = sess.applyBlock(&step)
		case "attestation":
			stepErr = sess.applyAttestation(&step)
		case "gossipAggregatedAttestation":
			stepErr = sess.applyAggregatedAttestation(&step)
		case "checks":
		default:
			stepErr = fmt.Errorf("unknown stepType %q", step.StepType)
		}

		// Safe target mutates the protoarray with the supermajority threshold, which
		// would corrupt the next step's head deltas. Compute it only when the step
		// actually checks it, and after the head is already stored for this step.
		if stepChecksSafeTarget(&step) {
			sess.refreshSafeTarget()
		}
		snap := sess.loadSnapshot()
		accepted := stepErr == nil
		var errPtr *string
		if stepErr != nil {
			msg := stepErr.Error()
			errPtr = &msg
		}
		writeJSON(w, http.StatusOK, driverStepResponse{Accepted: accepted, Error: errPtr, Snapshot: snap})
	}
}
