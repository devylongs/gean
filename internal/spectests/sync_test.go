//go:build spectests

package spectests

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

// Sync fixtures cover the spec's verify_checkpoint_state operation. Each
// .json carries one SSZ-encoded State plus a boolean indicating whether the
// structural rules accept it.
const syncFixturesRoot = "../../leanSpec/fixtures/consensus/sync/lstar/sync"

type syncFixtureOuter map[string]syncFixture

type syncFixture struct {
	Network   string                 `json:"network"`
	LeanEnv   string                 `json:"leanEnv"`
	Operation syncOperation          `json:"operation"`
	Output    syncOutput             `json:"output"`
	Info      map[string]interface{} `json:"_info"`
}

// operation carries the kind and its inputs inline (lstar fixture shape).
type syncOperation struct {
	Kind          string `json:"kind"`
	NumValidators uint64 `json:"numValidators"`
	AnchorSlot    uint64 `json:"anchorSlot"`
}

type syncOutput struct {
	Valid          bool   `json:"valid"`
	StateBytes     string `json:"stateBytes"`
	ValidatorCount uint64 `json:"validatorCount"`
	AnchorSlot     uint64 `json:"anchorSlot"`
}

// verifyCheckpointStructural mirrors the spec's verify_checkpoint_state:
// a state passes if it has at least one validator, the count stays inside
// VALIDATOR_REGISTRY_LIMIT,
// and its SSZ tree root can be computed (covers structural decode sanity).
//
// Production gean uses checkpoint.VerifyCheckpointState, which additionally
// cross-checks genesis time and the expected validator set against the
// caller-supplied configuration and rejects slot 0 outright. That broader
// policy is stricter than the spec's structural-only definition, so the
// spec test exercises the narrower contract here.
func verifyCheckpointStructural(state *types.State) bool {
	if len(state.Validators) == 0 {
		return false
	}
	if uint64(len(state.Validators)) > types.ValidatorRegistryLimit {
		return false
	}
	if _, err := state.HashTreeRoot(); err != nil {
		return false
	}
	return true
}

// TestSpecSync walks the sync fixture directory and, for each verify_checkpoint
// case, decodes the canonical stateBytes via gean's SSZ and asserts the
// structural verdict matches the spec-generated boolean. Also pins the
// extracted validator count and anchor slot.
func TestSpecSync(t *testing.T) {
	if _, err := os.Stat(syncFixturesRoot); os.IsNotExist(err) {
		t.Skipf("fixtures not present at %s; run 'make leanSpec/fixtures'", syncFixturesRoot)
	}

	err := filepath.Walk(syncFixturesRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: read: %v", path, err)
			return nil
		}

		var outer syncFixtureOuter
		if err := json.Unmarshal(raw, &outer); err != nil {
			t.Errorf("%s: parse: %v", path, err)
			return nil
		}

		base := strings.TrimSuffix(filepath.Base(path), ".json")
		for _, fx := range outer {
			fx := fx
			t.Run(base, func(t *testing.T) {
				if fx.Operation.Kind != "verify_checkpoint" {
					t.Skipf("unsupported sync operation %q", fx.Operation.Kind)
				}

				stateBytes, err := hex.DecodeString(strings.TrimPrefix(fx.Output.StateBytes, "0x"))
				if err != nil {
					t.Fatalf("decode stateBytes hex: %v", err)
				}

				state := &types.State{}
				ssz_err := state.UnmarshalSSZ(stateBytes)
				// A successful decode is a prerequisite for any structural check;
				// if the spec said valid=true, decode must succeed.
				if fx.Output.Valid && ssz_err != nil {
					t.Fatalf("expected SSZ decode to succeed for valid=true case: %v", ssz_err)
				}
				if ssz_err != nil {
					// Decode failure is itself a structural rejection; nothing
					// further to assert beyond the expected valid=false.
					if fx.Output.Valid {
						t.Fatalf("decode failed but spec marks valid=true: %v", ssz_err)
					}
					return
				}

				got := verifyCheckpointStructural(state)
				if got != fx.Output.Valid {
					t.Errorf("verifyCheckpointStructural = %v, want %v (validators=%d anchor_slot=%d)",
						got, fx.Output.Valid, len(state.Validators), state.Slot)
				}

				if uint64(len(state.Validators)) != fx.Output.ValidatorCount {
					t.Errorf("validator count from SSZ = %d, fixture says %d",
						len(state.Validators), fx.Output.ValidatorCount)
				}
				if state.Slot != fx.Output.AnchorSlot {
					t.Errorf("anchor slot from SSZ = %d, fixture says %d",
						state.Slot, fx.Output.AnchorSlot)
				}
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk fixtures: %v", err)
	}
}
