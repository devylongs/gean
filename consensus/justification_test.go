package consensus

import "testing"

func TestIsJustifiableAfter(t *testing.T) {
	tests := []struct {
		name      string
		finalized Slot
		candidate Slot
		expected  bool
	}{
		// Rule 1: delta <= 5
		{"delta_0", 10, 10, true},
		{"delta_1", 10, 11, true},
		{"delta_5", 10, 15, true},

		// Rule 2: perfect square
		{"delta_4_square", 10, 14, true},
		{"delta_9_square", 20, 29, true},
		{"delta_16_square", 50, 66, true},
		{"delta_100_square", 100, 200, true},

		// Rule 3: pronic number (x² + x)
		{"delta_6_pronic", 10, 16, true},
		{"delta_12_pronic", 20, 32, true},
		{"delta_20_pronic", 50, 70, true},
		{"delta_30_pronic", 100, 130, true},

		// Not justifiable
		{"delta_7_not_justifiable", 10, 17, false},
		{"delta_8_not_justifiable", 10, 18, false},
		{"delta_10_not_justifiable", 20, 30, false},
		{"delta_11_not_justifiable", 20, 31, false},
		{"delta_13_not_justifiable", 50, 63, false},
		{"delta_17_not_justifiable", 100, 117, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.candidate.IsJustifiableAfter(tt.finalized)
			if got != tt.expected {
				t.Errorf("Slot(%d).IsJustifiableAfter(%d) = %v, want %v",
					tt.candidate, tt.finalized, got, tt.expected)
			}
		})
	}
}

func TestIsJustifiableAfter_CandidateBeforeFinalized(t *testing.T) {
	// Should return false when candidate < finalized
	got := Slot(9).IsJustifiableAfter(Slot(10))
	if got != false {
		t.Errorf("expected false when candidate < finalized")
	}
}
