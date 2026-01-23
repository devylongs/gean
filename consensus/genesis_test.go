package consensus

import "testing"

func TestGenerateGenesis(t *testing.T) {
	genesisTime := uint64(1000000)
	numValidators := uint64(4096)

	state := GenerateGenesis(genesisTime, numValidators)

	// Check config
	if state.Config.GenesisTime != genesisTime {
		t.Errorf("GenesisTime = %d, want %d", state.Config.GenesisTime, genesisTime)
	}
	if state.Config.NumValidators != numValidators {
		t.Errorf("NumValidators = %d, want %d", state.Config.NumValidators, numValidators)
	}

	// Check initial state
	if state.Slot != 0 {
		t.Errorf("Slot = %d, want 0", state.Slot)
	}
	if state.LatestBlockHeader.Slot != 0 {
		t.Errorf("LatestBlockHeader.Slot = %d, want 0", state.LatestBlockHeader.Slot)
	}
	if !state.LatestJustified.Root.IsZero() {
		t.Error("LatestJustified.Root should be zero")
	}
	if !state.LatestFinalized.Root.IsZero() {
		t.Error("LatestFinalized.Root should be zero")
	}

	// Check empty lists
	if len(state.HistoricalBlockHashes) != 0 {
		t.Errorf("HistoricalBlockHashes len = %d, want 0", len(state.HistoricalBlockHashes))
	}
	if len(state.JustifiedSlots) != 0 {
		t.Errorf("JustifiedSlots len = %d, want 0", len(state.JustifiedSlots))
	}
}

func TestIsProposer(t *testing.T) {
	state := GenerateGenesis(0, 10)

	tests := []struct {
		slot      Slot
		validator ValidatorIndex
		expected  bool
	}{
		{0, 0, true},
		{0, 1, false},
		{1, 1, true},
		{1, 0, false},
		{10, 0, true},  // wraps around
		{15, 5, true},
	}

	for _, tt := range tests {
		state.Slot = tt.slot
		got := state.IsProposer(tt.validator)
		if got != tt.expected {
			t.Errorf("slot=%d, validator=%d: IsProposer = %v, want %v",
				tt.slot, tt.validator, got, tt.expected)
		}
	}
}
