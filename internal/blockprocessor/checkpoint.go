package blockprocessor

import (
	"fmt"

	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// justifiedCheckpointChange persists the post-state justified checkpoint when it
// advances. The finalized checkpoint is deliberately not advanced here: it is
// re-derived from the canonical head's chain during head selection, so a losing
// fork that finalized a higher slot cannot latch finalization above the head.
func justifiedCheckpointChange(s *store.ConsensusStore, postState *types.State) ([]storage.KV, error) {
	if s == nil || postState == nil {
		return nil, nil
	}
	if !checkpointAdvanced(postState.LatestJustified, s.LatestJustified()) {
		return nil, nil
	}
	entry, err := checkpointEntry(storage.KeyLatestJustified, postState.LatestJustified)
	if err != nil {
		return nil, err
	}
	return []storage.KV{entry}, nil
}

func checkpointAdvanced(candidate, current *types.Checkpoint) bool {
	if candidate == nil {
		return false
	}
	return current == nil || candidate.Slot > current.Slot
}

func checkpointEntry(key []byte, checkpoint *types.Checkpoint) (storage.KV, error) {
	data, err := checkpoint.MarshalSSZ()
	if err != nil {
		return storage.KV{}, fmt.Errorf("marshal checkpoint: %w", err)
	}
	return storage.KV{Key: key, Value: data}, nil
}
