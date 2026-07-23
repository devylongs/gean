//go:build spectests

package spectests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/statetransition"
)

func TestSpecStateTransition(t *testing.T) {
	logger.SetQuiet(true)
	defer logger.SetQuiet(false)

	fixtureDir := "../../leanSpec/fixtures/consensus/state_transition"

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

			var fixture TestFixture
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatalf("unmarshalling %s: %v", file, err)
			}

			for testName, tt := range fixture {
				tt := tt
				t.Run(testName, func(t *testing.T) {
					runStateTransitionTest(t, &tt)
				})
			}
		})
	}
}

func runStateTransitionTest(t *testing.T, tt *StateTransitionTest) {
	t.Helper()

	expectError := tt.ExpectException != "" || tt.Post == nil

	// Empty blocks list with an expected rejection means the spec filler raised
	// the assertion internally (e.g., process_slots target == state.slot rejected
	// before any block was constructed). The rejection may be signalled via
	// expectException or via rejectionReason with no post state. The spec
	// framework's verdict stands; nothing for us to replay.
	if len(tt.Blocks) == 0 && expectError {
		return
	}

	state := tt.Pre.ToState()

	var lastErr error
	for i, tb := range tt.Blocks {
		block := tb.ToBlock()
		if err := statetransition.StateTransition(state, block); err != nil {
			lastErr = err
			if expectError {
				t.Logf("block %d (slot %d): expected error: %v", i, tb.Slot, err)
				return
			}
			t.Fatalf("block %d (slot %d): unexpected error: %v", i, tb.Slot, err)
		}
	}

	if expectError && lastErr == nil {
		t.Fatal("expected error but all blocks processed successfully")
	}

	// No blocks and no error expected: genesis validation test.
	if tt.Post != nil {
		if err := tt.Post.Validate(state); err != nil {
			t.Fatalf("post-state validation failed: %v", err)
		}
	}
}
