//go:build spectests

package spectests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/statetransition"
)

// Justifiability fixtures live under the lstar/state_transition path tree.
// Each .json file carries one (slot, finalizedSlot) -> isJustifiable case.
const justifiabilityFixturesRoot = "../../leanSpec/fixtures/consensus/justifiability/lstar/state_transition/test_justifiability"

type justifiabilityFixtureOuter map[string]justifiabilityFixture

type justifiabilityFixture struct {
	Network       string                 `json:"network"`
	LeanEnv       string                 `json:"leanEnv"`
	Slot          uint64                 `json:"slot"`
	FinalizedSlot uint64                 `json:"finalizedSlot"`
	Output        justifiabilityOutput   `json:"output"`
	Info          map[string]interface{} `json:"_info"`
}

type justifiabilityOutput struct {
	// delta = slot - finalizedSlot, signed: a slot before the finalized boundary
	// yields a negative delta.
	Delta         int64 `json:"delta"`
	IsJustifiable bool  `json:"isJustifiable"`
}

// TestSpecJustifiability walks the justifiability fixture directory and asserts
// statetransition.SlotIsJustifiableAfter matches the spec-generated boolean for
// every (slot, finalizedSlot) pair. Each fixture also carries the precomputed
// delta = slot - finalizedSlot, which we cross-check for fixture integrity.
func TestSpecJustifiability(t *testing.T) {
	if _, err := os.Stat(justifiabilityFixturesRoot); os.IsNotExist(err) {
		t.Skipf("fixtures not present at %s; run 'make leanSpec/fixtures'", justifiabilityFixturesRoot)
	}

	err := filepath.Walk(justifiabilityFixturesRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: read: %v", path, err)
			return nil
		}

		var outer justifiabilityFixtureOuter
		if err := json.Unmarshal(raw, &outer); err != nil {
			t.Errorf("%s: parse: %v", path, err)
			return nil
		}

		base := strings.TrimSuffix(filepath.Base(path), ".json")
		for _, fx := range outer {
			fx := fx
			t.Run(base, func(t *testing.T) {
				expectedDelta := int64(fx.Slot) - int64(fx.FinalizedSlot)
				if fx.Output.Delta != expectedDelta {
					t.Errorf("fixture delta inconsistent: slot=%d finalizedSlot=%d computedDelta=%d fixtureDelta=%d",
						fx.Slot, fx.FinalizedSlot, expectedDelta, fx.Output.Delta)
				}

				got := statetransition.SlotIsJustifiableAfter(fx.Slot, fx.FinalizedSlot)
				if got != fx.Output.IsJustifiable {
					t.Errorf("SlotIsJustifiableAfter(slot=%d, finalizedSlot=%d) = %v, want %v (delta=%d)",
						fx.Slot, fx.FinalizedSlot, got, fx.Output.IsJustifiable, expectedDelta)
				}
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk fixtures: %v", err)
	}
}
