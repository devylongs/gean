package node

import (
	"fmt"
	"sort"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/store"
)

// ForkChoiceFromStore anchors fork choice at the latest justified block and
// replays every processed block above it. FindHead descends from the
// justified root, so after a restart that root must be present in the array —
// anchoring at the bare DB head leaves justified as an unknown ancestor and
// pins the head there permanently. All branches are replayed, not just the
// head chain: the network may have built on a sibling the previous run never
// chose as head, and a missing fork point leaves that subtree dangling and
// weightless. Only blocks with a stored post-state qualify — headers are also
// persisted for pending blocks that were never verified, and an unverified
// block must not be able to win head.
func ForkChoiceFromStore(s *store.ConsensusStore) (*forkchoice.ForkChoice, error) {
	if s == nil {
		return nil, fmt.Errorf("fork choice anchor: store is nil")
	}
	anchorRoot := s.Head()
	if justified := s.LatestJustified(); justified != nil && s.GetBlockHeader(justified.Root) != nil {
		anchorRoot = justified.Root
	}
	anchorHeader := s.GetBlockHeader(anchorRoot)
	if anchorHeader == nil {
		return nil, fmt.Errorf("fork choice anchor: missing header for root 0x%x", anchorRoot)
	}
	fc := forkchoice.New(anchorHeader.Slot, anchorRoot, anchorHeader.ParentRoot)

	roots, err := s.BlockRoots()
	if err != nil {
		return nil, fmt.Errorf("fork choice anchor: %w", err)
	}
	type replayEntry struct {
		slot         uint64
		root, parent [32]byte
	}
	replay := make([]replayEntry, 0, len(roots))
	for root := range roots {
		header := s.GetBlockHeader(root)
		if header == nil || header.Slot <= anchorHeader.Slot || !s.HasState(root) {
			continue
		}
		replay = append(replay, replayEntry{header.Slot, root, header.ParentRoot})
	}
	sort.Slice(replay, func(i, j int) bool { return replay[i].slot < replay[j].slot })
	for _, e := range replay {
		fc.OnBlock(e.slot, e.root, e.parent)
	}
	return fc, nil
}
