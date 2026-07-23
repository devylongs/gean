//go:build spectests

package spectests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

// Slot-clock fixtures cover these five derivations:
//   - current_slot, current_interval, total_intervals: f(genesis_time, now_ms)
//   - from_slot:                                       f(slot)
//   - from_unix_time:                                  f(unix_seconds, genesis_time)
const slotClockFixturesRoot = "../../leanSpec/fixtures/consensus/slot_clock/lstar/chain/test_slot_clock"

type slotClockFixtureOuter map[string]slotClockFixture

type slotClockFixture struct {
	Network   string                 `json:"network"`
	LeanEnv   string                 `json:"leanEnv"`
	Operation slotClockOperation     `json:"operation"`
	Config    slotClockConfig        `json:"config"`
	Output    slotClockOutput        `json:"output"`
	Info      map[string]interface{} `json:"_info"`
}

// operation carries the derivation kind and its inputs inline. The spec emits
// the numeric inputs as JSON floats, so they are parsed as floats and truncated.
type slotClockOperation struct {
	Kind                    string  `json:"kind"`
	GenesisTime             float64 `json:"genesisTime"`
	CurrentTimeMilliseconds float64 `json:"currentTimeMilliseconds"`
	UnixSeconds             float64 `json:"unixSeconds"`
	Slot                    float64 `json:"slot"`
}

type slotClockOutput struct {
	Slot           uint64 `json:"slot"`
	Interval       uint64 `json:"interval"`
	TotalIntervals uint64 `json:"totalIntervals"`
}

type slotClockConfig struct {
	SecondsPerSlot          uint64 `json:"secondsPerSlot"`
	IntervalsPerSlot        uint64 `json:"intervalsPerSlot"`
	MillisecondsPerInterval uint64 `json:"millisecondsPerInterval"`
}

// TestSpecSlotClock walks the slot_clock fixture directory and exercises
// each of the five slot-clock derivations against the canonical
// spec-generated inputs and outputs.
//
// Also pins the timing config (seconds/slot, intervals/slot, ms/interval)
// from the fixture against gean's constants to catch drift in either
// direction.
func TestSpecSlotClock(t *testing.T) {
	if _, err := os.Stat(slotClockFixturesRoot); os.IsNotExist(err) {
		t.Skipf("fixtures not present at %s; run 'make leanSpec/fixtures'", slotClockFixturesRoot)
	}

	err := filepath.Walk(slotClockFixturesRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: read: %v", path, err)
			return nil
		}

		var outer slotClockFixtureOuter
		if err := json.Unmarshal(raw, &outer); err != nil {
			t.Errorf("%s: parse: %v", path, err)
			return nil
		}

		base := strings.TrimSuffix(filepath.Base(path), ".json")
		for _, fx := range outer {
			fx := fx
			t.Run(base, func(t *testing.T) {
				assertConstantsMatch(t, fx.Config)

				op := fx.Operation
				gt := uint64(op.GenesisTime)
				nowMs := uint64(op.CurrentTimeMilliseconds)
				unix := uint64(op.UnixSeconds)
				slot := uint64(op.Slot)
				switch op.Kind {
				case "current_slot":
					got := types.CurrentSlot(gt, nowMs)
					if got != fx.Output.Slot {
						t.Errorf("CurrentSlot(gt=%d, now=%d) = %d, want %d", gt, nowMs, got, fx.Output.Slot)
					}
				case "current_interval":
					got := types.CurrentInterval(gt, nowMs)
					if got != fx.Output.Interval {
						t.Errorf("CurrentInterval(gt=%d, now=%d) = %d, want %d", gt, nowMs, got, fx.Output.Interval)
					}
				case "total_intervals":
					got := types.TotalIntervals(gt, nowMs)
					if got != fx.Output.TotalIntervals {
						t.Errorf("TotalIntervals(gt=%d, now=%d) = %d, want %d", gt, nowMs, got, fx.Output.TotalIntervals)
					}
				case "from_slot":
					got := types.IntervalsFromSlot(slot)
					if got != fx.Output.Interval {
						t.Errorf("IntervalsFromSlot(%d) = %d, want %d", slot, got, fx.Output.Interval)
					}
				case "from_unix_time":
					got := types.IntervalsFromUnixTime(unix, gt)
					if got != fx.Output.Interval {
						t.Errorf("IntervalsFromUnixTime(unix=%d, gt=%d) = %d, want %d", unix, gt, got, fx.Output.Interval)
					}
				default:
					t.Skipf("unsupported slot_clock operation %q", op.Kind)
				}
			})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk fixtures: %v", err)
	}
}

// assertConstantsMatch confirms that the fixture's declared timing config
// matches gean's compiled-in constants. A mismatch here means the network
// timing has drifted between spec and gean — every per-fixture assertion
// below would still pass numerically but with the wrong physical meaning.
func assertConstantsMatch(t *testing.T, cfg slotClockConfig) {
	t.Helper()
	if cfg.SecondsPerSlot != types.SecondsPerSlot {
		t.Fatalf("fixture secondsPerSlot=%d, gean SecondsPerSlot=%d",
			cfg.SecondsPerSlot, types.SecondsPerSlot)
	}
	if cfg.IntervalsPerSlot != types.IntervalsPerSlot {
		t.Fatalf("fixture intervalsPerSlot=%d, gean IntervalsPerSlot=%d",
			cfg.IntervalsPerSlot, types.IntervalsPerSlot)
	}
	if cfg.MillisecondsPerInterval != types.MillisecondsPerInterval {
		t.Fatalf("fixture millisecondsPerInterval=%d, gean MillisecondsPerInterval=%d",
			cfg.MillisecondsPerInterval, types.MillisecondsPerInterval)
	}
}
