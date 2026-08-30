package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func makeChainStore(t *testing.T, slots []uint64) (*store.ConsensusStore, map[uint64][32]byte) {
	t.Helper()
	s := makeTestStore()

	roots := make(map[uint64][32]byte)
	var prevRoot [32]byte
	for _, slot := range slots {
		var root [32]byte
		root[0] = byte(slot + 1)
		signed := &types.SignedBlock{
			Block: &types.Block{
				Slot:       slot,
				ParentRoot: prevRoot,
				Body:       &types.BlockBody{},
			},
		}
		s.StorePendingBlock(root, signed)
		roots[slot] = root
		prevRoot = root
	}
	if len(slots) > 0 {
		s.SetHead(roots[slots[len(slots)-1]])
	}
	return s, roots
}

func TestGetCanonicalBlocksInRange_FullChain(t *testing.T) {
	s, roots := makeChainStore(t, []uint64{0, 2, 5, 7})

	got, _ := s.GetCanonicalBlocksInRange(0, 8)
	if len(got) != 4 {
		t.Fatalf("expected 4 blocks, got %d", len(got))
	}
	wantSlots := []uint64{0, 2, 5, 7}
	for i, b := range got {
		if b.Block.Slot != wantSlots[i] {
			t.Errorf("blocks[%d].Slot = %d, want %d", i, b.Block.Slot, wantSlots[i])
		}
	}
	_ = roots
}

func TestGetCanonicalBlocksInRange_ZeroCount(t *testing.T) {
	s, _ := makeChainStore(t, []uint64{0, 1, 2})
	if got, _ := s.GetCanonicalBlocksInRange(0, 0); got != nil {
		t.Fatalf("expected nil for count=0, got %d blocks", len(got))
	}
}

func TestGetCanonicalBlocksInRange_SkipsEmptySlots(t *testing.T) {
	s, _ := makeChainStore(t, []uint64{0, 2, 5, 7})

	got, _ := s.GetCanonicalBlocksInRange(3, 4)
	if len(got) != 1 || got[0].Block.Slot != 5 {
		t.Fatalf("expected single block at slot 5, got %d blocks", len(got))
	}

	got, _ = s.GetCanonicalBlocksInRange(2, 4)
	if len(got) != 2 || got[0].Block.Slot != 2 || got[1].Block.Slot != 5 {
		t.Fatalf("expected blocks at [2, 5], got %d blocks", len(got))
	}
}

func TestGetCanonicalBlocksInRange_AboveHead(t *testing.T) {
	s, _ := makeChainStore(t, []uint64{0, 2, 5, 7})

	got, _ := s.GetCanonicalBlocksInRange(10, 5)
	if got != nil {
		t.Fatalf("expected nil for range above head, got %d blocks", len(got))
	}
}

func TestGetCanonicalBlocksInRange_AscendingOrder(t *testing.T) {
	s, _ := makeChainStore(t, []uint64{0, 1, 2, 3, 4, 5})

	got, _ := s.GetCanonicalBlocksInRange(1, 4)
	if len(got) != 4 {
		t.Fatalf("expected 4 blocks, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Block.Slot <= got[i-1].Block.Slot {
			t.Fatalf("non-ascending slots at index %d: %d <= %d",
				i, got[i].Block.Slot, got[i-1].Block.Slot)
		}
	}
}

func TestGetCanonicalBlocksInRange_PartialFromGenesis(t *testing.T) {
	s, _ := makeChainStore(t, []uint64{0, 1, 2, 3, 4, 5, 6, 7})

	got, _ := s.GetCanonicalBlocksInRange(0, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 blocks, got %d", len(got))
	}
	for i, b := range got {
		if b.Block.Slot != uint64(i) {
			t.Errorf("blocks[%d].Slot = %d, want %d", i, b.Block.Slot, i)
		}
	}
}

// A chain that does not reach back to startSlot is not an empty range: the walk
// runs out of ancestors (a checkpoint-synced node below its anchor), and the
// caller must be able to tell the two apart.
func TestGetCanonicalBlocksInRange_ReportsIncompleteChain(t *testing.T) {
	tests := []struct {
		name         string
		chain        []uint64
		startSlot    uint64
		count        uint64
		wantBlocks   int
		wantComplete bool
	}{
		{"reaches genesis", []uint64{0, 1, 2, 3}, 0, 4, 4, true},
		{"walk stops below startSlot", []uint64{0, 1, 2, 3, 4, 5}, 3, 3, 3, true},
		{"ancestors end above startSlot", []uint64{5, 6, 7}, 0, 8, 3, false},
		{"anchor above requested range", []uint64{5, 6, 7}, 1, 2, 0, false},
		{"zero count", []uint64{0, 1, 2}, 0, 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := makeChainStore(t, tt.chain)

			got, complete := s.GetCanonicalBlocksInRange(tt.startSlot, tt.count)
			if len(got) != tt.wantBlocks {
				t.Errorf("blocks = %d, want %d", len(got), tt.wantBlocks)
			}
			if complete != tt.wantComplete {
				t.Errorf("complete = %v, want %v", complete, tt.wantComplete)
			}
		})
	}
}
