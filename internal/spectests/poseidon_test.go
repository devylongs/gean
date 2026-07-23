//go:build spectests

package spectests

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/geanlabs/gean/xmss"
)

type poseidonCase struct {
	Width       int      `json:"width"`
	InputState  []string `json:"inputState"`
	OutputState []string `json:"outputState"`
}

func TestSpecPoseidonPermutation(t *testing.T) {
	walkFixtures(t, "../../leanSpec/fixtures/consensus/poseidon_permutation", func(t *testing.T, raw []byte) {
		var fixture map[string]poseidonCase
		if err := json.Unmarshal(raw, &fixture); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		for name, tc := range fixture {
			tc := tc
			t.Run(name, func(t *testing.T) {
				state := parseFieldElements(t, tc.InputState)
				if err := xmss.Poseidon2Permute(state); err != nil {
					t.Fatalf("permute width %d: %v", tc.Width, err)
				}
				want := parseFieldElements(t, tc.OutputState)
				if len(state) != len(want) {
					t.Fatalf("length mismatch: got %d want %d", len(state), len(want))
				}
				for i := range want {
					if state[i] != want[i] {
						t.Fatalf("element %d: got %d want %d", i, state[i], want[i])
					}
				}
			})
		}
	})
}

func parseFieldElements(t *testing.T, vals []string) []uint32 {
	t.Helper()
	out := make([]uint32, len(vals))
	for i, v := range vals {
		n, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			t.Fatalf("parse field element %q: %v", v, err)
		}
		out[i] = uint32(n)
	}
	return out
}
