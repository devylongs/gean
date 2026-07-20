//go:build hive_testdriver

package testdriver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/geanlabs/gean/internal/specfixtures"
)

// TestForkChoiceReplayLocalFixtures drives the local fork-choice spec fixtures
// through the HTTP step handlers exactly as the hive simulator does, then asserts
// the returned snapshot with label resolution — the same checks the local
// spectests harness makes. It is the fast, hive-free guard that the driver's
// step semantics match the fork-choice engine the local harness already proves
// correct. Skips when the fixtures have not been generated.
func TestForkChoiceReplayLocalFixtures(t *testing.T) {
	root := forkChoiceFixtureRoot(t)
	files := collectJSON(t, root)
	if len(files) == 0 {
		t.Skipf("no fork-choice fixtures under %s (run make test-spec)", root)
	}

	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for name, caseRaw := range wrapper {
			name := name
			caseRaw := caseRaw
			t.Run(shortName(file, name), func(t *testing.T) {
				runReplayCase(t, caseRaw)
			})
		}
	}
}

type replayFixture struct {
	AnchorState json.RawMessage   `json:"anchorState"`
	AnchorBlock json.RawMessage   `json:"anchorBlock"`
	Steps       []json.RawMessage `json:"steps"`
}

func runReplayCase(t *testing.T, caseRaw json.RawMessage) {
	var rc replayFixture
	if err := json.Unmarshal(caseRaw, &rc); err != nil {
		t.Fatalf("decode case: %v", err)
	}

	sess := NewSession()

	// Init.
	genesisTime := pointerU64(caseRaw, "anchorState", "config", "genesisTime")
	init := map[string]any{"anchorState": rc.AnchorState, "anchorBlock": rc.AnchorBlock}
	if genesisTime != nil {
		init["genesisTime"] = *genesisTime
	}
	initRec := post(t, sess.ForkChoiceInitHandler(), init)
	if len(rc.Steps) == 0 && initRec.Code != http.StatusNoContent {
		return // negative anchor fixture: init rejection is the expected outcome
	}
	if initRec.Code != http.StatusNoContent {
		t.Fatalf("init failed: status=%d body=%s", initRec.Code, initRec.Body.String())
	}

	// Resolve label -> root and root -> slot on the test side, mirroring the
	// blockRootLabel the fixtures reference in their checks.
	labelToRoot := map[string]string{}
	rootToSlot := map[string]uint64{}
	if r, s := anchorRootSlot(t, rc.AnchorBlock); r != "" {
		rootToSlot[r] = s
	}

	for i, stepRaw := range rc.Steps {
		var step specfixtures.ForkChoiceStep
		if err := json.Unmarshal(stepRaw, &step); err != nil {
			t.Fatalf("step %d decode: %v", i, err)
		}
		if step.StepType == "block" && step.Block != nil {
			if root, slot, ok := blockRootSlot(step.Block); ok {
				rootToSlot[root] = slot
				if step.Block.BlockRootLabel != "" {
					labelToRoot[step.Block.BlockRootLabel] = root
				}
			}
		}

		var resp driverStepResponse
		rec := post(t, sess.ForkChoiceStepHandler(), stepRaw)
		if rec.Code != http.StatusOK {
			t.Fatalf("step %d: status=%d body=%s", i, rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("step %d decode response: %v", i, err)
		}

		// Acceptance: the driver reports actual acceptance; it must match the
		// fixture's declared expectation.
		if hasField(stepRaw, "valid") && resp.Accepted != step.Valid {
			t.Fatalf("step %d (%s) acceptance: got %v want %v (err=%v)",
				i, step.StepType, resp.Accepted, step.Valid, resp.Error)
		}
		if !resp.Accepted {
			continue
		}
		if step.Checks != nil {
			assertChecks(t, i, step.Checks, resp.Snapshot, labelToRoot, rootToSlot)
		}
	}
}

func assertChecks(t *testing.T, idx int, c *specfixtures.FCChecks, snap driverSnapshot, labelToRoot map[string]string, rootToSlot map[string]uint64) {
	if c.HeadSlot != nil && snap.HeadSlot != *c.HeadSlot {
		t.Fatalf("step %d headSlot: got %d want %d", idx, snap.HeadSlot, *c.HeadSlot)
	}
	if c.HeadRoot != nil && norm(snap.HeadRoot) != norm(*c.HeadRoot) {
		t.Fatalf("step %d headRoot: got %s want %s", idx, norm(snap.HeadRoot), norm(*c.HeadRoot))
	}
	if c.HeadRootLabel != nil {
		if want, ok := labelToRoot[*c.HeadRootLabel]; ok && norm(snap.HeadRoot) != norm(want) {
			t.Fatalf("step %d headRootLabel %q: got %s want %s", idx, *c.HeadRootLabel, norm(snap.HeadRoot), norm(want))
		}
	}
	if c.Time != nil && snap.Time != *c.Time {
		t.Fatalf("step %d time: got %d want %d", idx, snap.Time, *c.Time)
	}
	if c.LatestJustifiedSlot != nil && snap.JustifiedCheckpoint.Slot != *c.LatestJustifiedSlot {
		t.Fatalf("step %d justifiedSlot: got %d want %d", idx, snap.JustifiedCheckpoint.Slot, *c.LatestJustifiedSlot)
	}
	if c.LatestJustifiedRootLabel != nil {
		if want, ok := labelToRoot[*c.LatestJustifiedRootLabel]; ok && norm(snap.JustifiedCheckpoint.Root) != norm(want) {
			t.Fatalf("step %d justifiedRootLabel %q: got %s want %s", idx, *c.LatestJustifiedRootLabel, norm(snap.JustifiedCheckpoint.Root), norm(want))
		}
	}
	if c.LatestFinalizedSlot != nil && snap.FinalizedCheckpoint.Slot != *c.LatestFinalizedSlot {
		t.Fatalf("step %d finalizedSlot: got %d want %d", idx, snap.FinalizedCheckpoint.Slot, *c.LatestFinalizedSlot)
	}
	if c.LatestFinalizedRootLabel != nil {
		if want, ok := labelToRoot[*c.LatestFinalizedRootLabel]; ok && norm(snap.FinalizedCheckpoint.Root) != norm(want) {
			t.Fatalf("step %d finalizedRootLabel %q: got %s want %s", idx, *c.LatestFinalizedRootLabel, norm(snap.FinalizedCheckpoint.Root), norm(want))
		}
	}
	if c.SafeTarget != nil && norm(snap.SafeTarget) != norm(*c.SafeTarget) {
		t.Fatalf("step %d safeTarget: got %s want %s", idx, norm(snap.SafeTarget), norm(*c.SafeTarget))
	}
	if c.SafeTargetRootLabel != nil {
		if want, ok := labelToRoot[*c.SafeTargetRootLabel]; ok && norm(snap.SafeTarget) != norm(want) {
			t.Fatalf("step %d safeTargetRootLabel %q: got %s want %s", idx, *c.SafeTargetRootLabel, norm(snap.SafeTarget), norm(want))
		}
	}
	if c.SafeTargetSlot != nil {
		if got, ok := rootToSlot[norm(snap.SafeTarget)]; ok && got != *c.SafeTargetSlot {
			t.Fatalf("step %d safeTargetSlot: got %d want %d", idx, got, *c.SafeTargetSlot)
		}
	}
}

// --- helpers ---

func post(t *testing.T, h http.HandlerFunc, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	switch p := payload.(type) {
	case json.RawMessage:
		body = p
	default:
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		body = b
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func blockRootSlot(b *specfixtures.TestBlock) (string, uint64, bool) {
	block, err := b.ToBlock()
	if err != nil {
		return "", 0, false
	}
	root, err := block.HashTreeRoot()
	if err != nil {
		return "", 0, false
	}
	return norm(hex(root[:])), block.Slot, true
}

func anchorRootSlot(t *testing.T, anchorRaw json.RawMessage) (string, uint64) {
	var tb specfixtures.TestBlock
	if err := json.Unmarshal(anchorRaw, &tb); err != nil {
		return "", 0
	}
	r, s, ok := blockRootSlot(&tb)
	if !ok {
		return "", 0
	}
	return r, s
}

func norm(s string) string { return strings.ToLower(strings.TrimPrefix(s, "0x")) }

func hex(b []byte) string {
	const h = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = h[c>>4]
		out[i*2+1] = h[c&0xf]
	}
	return string(out)
}

func hasField(raw json.RawMessage, key string) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

func pointerU64(raw json.RawMessage, path ...string) *uint64 {
	cur := raw
	for _, k := range path {
		var m map[string]json.RawMessage
		if json.Unmarshal(cur, &m) != nil {
			return nil
		}
		next, ok := m[k]
		if !ok {
			return nil
		}
		cur = next
	}
	var v uint64
	if json.Unmarshal(cur, &v) != nil {
		return nil
	}
	return &v
}

func forkChoiceFixtureRoot(t *testing.T) string {
	t.Helper()
	// package dir internal/api/testdriver -> repo root is three levels up.
	return filepath.Join("..", "..", "..", "leanSpec", "fixtures", "consensus", "fork_choice", "lstar", "fork_choice")
}

func collectJSON(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".json") {
			out = append(out, path)
		}
		return nil
	})
	return out
}

func shortName(file, testName string) string {
	base := strings.TrimSuffix(filepath.Base(file), ".json")
	if i := strings.Index(testName, "::"); i >= 0 {
		testName = testName[i+2:]
	}
	if i := strings.Index(testName, "["); i >= 0 {
		testName = testName[:i]
	}
	return base + "/" + testName
}
