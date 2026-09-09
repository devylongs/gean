package node

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

// Finalization pruning is the only thing that shrinks TableStates and
// TableBlockHeaders, and it reads its delete list from fork choice. For a long
// time it deleted nothing at all: updateFinalizedFromHead pruned the ProtoArray
// before calling PruneOnFinalization, so GetCanonicalAnalysis ran against an
// array that no longer held the ancestors or the losing branches and returned
// two empty lists. Nothing failed, nothing logged, and the two tables grew for
// the life of the chain — which is what made the per-slot full-table scans on
// the dispatch loop expensive enough to stall the slot clock.
//
// The shape here is: genesis -> a(1) -> b(2) -> c(3), with a losing fork x(2)
// off a. Finalizing b must drop x entirely and drop a's state, while keeping
// the finalized block's own state and everything above it.
func TestFinalizationPrunesAncestorAndLosingBranch(t *testing.T) {
	e := makeTestEngine()

	genesis := [32]byte{0x01}
	rootA := [32]byte{0x0a}
	rootB := [32]byte{0x0b}
	rootC := [32]byte{0x0c}
	rootX := [32]byte{0x0f}

	// finalizedIn is the checkpoint a block's post-state carries.
	add := func(root, parent [32]byte, slot uint64, finalizedIn *types.Checkpoint) {
		e.Store.InsertBlockHeader(root, &types.BlockHeader{Slot: slot, ParentRoot: parent})
		e.Store.InsertState(root, &types.State{
			Slot:                     slot,
			LatestBlockHeader:        &types.BlockHeader{Slot: slot, ParentRoot: parent},
			LatestJustified:          &types.Checkpoint{Root: genesis, Slot: 0},
			LatestFinalized:          finalizedIn,
			JustifiedSlots:           types.NewBitlistSSZ(0),
			JustificationsValidators: types.NewBitlistSSZ(0),
		})
		e.FC.OnBlock(slot, root, parent)
	}

	genesisCP := &types.Checkpoint{Root: genesis, Slot: 0}
	add(rootA, genesis, 1, genesisCP)
	add(rootX, rootA, 2, genesisCP)
	add(rootB, rootA, 2, genesisCP)
	// c's post-state is what drives finalization: it finalizes b at slot 2.
	add(rootC, rootB, 3, &types.Checkpoint{Root: rootB, Slot: 2})

	e.Store.SetHead(rootC)
	e.updateFinalizedFromHead(rootC)

	if got := e.Store.LatestFinalized(); got == nil || got.Slot != 2 || got.Root != rootB {
		t.Fatalf("finalized checkpoint = %+v, want slot 2 root %x", got, rootB)
	}

	// The losing branch goes entirely: state and block header both.
	if e.Store.HasState(rootX) {
		t.Error("losing branch state survived finalization pruning")
	}
	if e.Store.GetBlockHeader(rootX) != nil {
		t.Error("losing branch block header survived finalization pruning")
	}

	// Ancestors below the finalized root keep their headers but lose their
	// states — canonical[1:] in PruneOnFinalization.
	if e.Store.HasState(rootA) {
		t.Error("ancestor state below the finalized root survived finalization pruning")
	}

	// The finalized block and the head above it must be untouched: without
	// their states the node cannot run a transition from the finalized anchor.
	if !e.Store.HasState(rootB) {
		t.Error("finalized block state was pruned")
	}
	if !e.Store.HasState(rootC) {
		t.Error("head state was pruned")
	}
}
