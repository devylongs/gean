package shadow

import (
	"testing"
	"time"
)

// A non-positive rate or zero unit count must be a true no-op so production
// nodes (rates unset) never pay any delay.
func TestSleepDisabled(t *testing.T) {
	cases := []struct {
		name  string
		rate  float64
		units int
	}{
		{"zero rate", 0, 1_000_000},
		{"negative rate", -1, 1_000_000},
		{"zero units", 1, 0},
		{"negative units", 1, -5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			sleep(tc.rate, tc.units)
			if elapsed := time.Since(start); elapsed > time.Millisecond {
				t.Fatalf("expected no-op, slept %v", elapsed)
			}
		})
	}
}

// A positive rate sleeps the n/rate floor: more units cost more virtual time.
func TestSleepScalesWithUnits(t *testing.T) {
	const rate = 1000.0 // 1000 signature-units per second → 1ms each
	start := time.Now()
	sleep(rate, 5) // expect ~5ms
	if elapsed := time.Since(start); elapsed < 4*time.Millisecond {
		t.Fatalf("expected at least ~5ms, slept %v", elapsed)
	}
}
