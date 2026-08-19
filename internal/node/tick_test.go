package node

import "testing"

// TestEarlyAggregationQuorum pins the early-dispatch threshold to the 3SF
// supermajority ceil(2n/3). An off-by-one here would either fire the early
// session before enough votes are in (poor coverage) or never fire it (no head
// start), so the boundary is verified against hand-computed ceil(2n/3).
func TestEarlyAggregationQuorum(t *testing.T) {
	cases := []struct {
		n    uint64
		want int
	}{
		{0, 0},
		{1, 1},
		{2, 2},
		{3, 2},
		{4, 3},
		{5, 4},
		{10, 7},
		{100, 67},
	}
	for _, c := range cases {
		if got := earlyAggregationQuorum(c.n); got != c.want {
			t.Errorf("earlyAggregationQuorum(%d)=%d, want %d", c.n, got, c.want)
		}
	}
}
