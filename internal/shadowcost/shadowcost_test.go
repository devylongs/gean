package shadowcost

import (
	"testing"
	"time"
)

func TestEnabled(t *testing.T) {
	cases := []struct {
		name string
		c    Costs
		want bool
	}{
		{"zero", Costs{}, false},
		{"aggregate", Costs{Aggregate: time.Millisecond}, true},
		{"verify", Costs{Verify: time.Millisecond}, true},
	}
	for _, tc := range cases {
		if got := tc.c.Enabled(); got != tc.want {
			t.Errorf("%s: Enabled()=%v want %v", tc.name, got, tc.want)
		}
	}
}

func TestDelayZeroIsNoOp(t *testing.T) {
	start := time.Now()
	c := Costs{}
	c.DelayAggregate()
	c.DelayVerify()
	if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
		t.Errorf("zero-cost delays should not block, took %v", elapsed)
	}
}

func TestDelaySleeps(t *testing.T) {
	c := Costs{Aggregate: 20 * time.Millisecond}
	start := time.Now()
	c.DelayAggregate()
	if elapsed := time.Since(start); elapsed < 15*time.Millisecond {
		t.Errorf("configured delay did not sleep, took %v", elapsed)
	}
}
