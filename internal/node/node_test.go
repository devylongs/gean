package node

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/syncer"
	"github.com/geanlabs/gean/internal/types"
)

func TestMain(m *testing.M) {
	logger.SetQuiet(true)
	os.Exit(m.Run())
}

func makeTestEngine() *Engine {
	backend := storage.NewInMemoryBackend()
	s := store.NewConsensusStore(backend)

	s.SetConfig(&types.ChainConfig{GenesisTime: 1000})
	var genesisRoot [32]byte
	genesisRoot[0] = 0x01
	s.SetHead(genesisRoot)
	s.SetSafeTarget(genesisRoot)
	s.SetLatestJustified(&types.Checkpoint{Root: genesisRoot, Slot: 0})
	s.SetLatestFinalized(&types.Checkpoint{Root: genesisRoot, Slot: 0})
	s.InsertBlockHeader(genesisRoot, &types.BlockHeader{Slot: 0})

	genesisState := &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     0,
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          &types.Checkpoint{Root: genesisRoot, Slot: 0},
		LatestFinalized:          &types.Checkpoint{Root: genesisRoot, Slot: 0},
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
	s.InsertState(genesisRoot, genesisState)

	fc := forkchoice.New(0, genesisRoot, [32]byte{})

	return New(s, fc, nil, nil, role.New(false), 1, shadow.Rates{})
}

func TestEngineCreation(t *testing.T) {
	e := makeTestEngine()
	if e.Store == nil {
		t.Fatal("store should not be nil")
	}
	if e.FC == nil {
		t.Fatal("fork choice should not be nil")
	}
}

func TestAcceptProposalRejectsStaleResult(t *testing.T) {
	e := makeTestEngine()
	e.Store.SetConfig(&types.ChainConfig{GenesisTime: 1})
	result := &proposalResult{
		blockRoot: [32]byte{0x02},
		signedBlock: &types.SignedBlock{
			Block: &types.Block{Slot: 1, Body: &types.BlockBody{}},
			Proof: &types.MultiMessageAggregate{Proof: []byte{1}},
		},
	}

	e.acceptProposal(context.Background(), result)

	if e.Store.HasState(result.blockRoot) {
		t.Fatal("stale proposal was imported")
	}
}

func TestCoversParticipants(t *testing.T) {
	proof := &types.SingleMessageAggregate{Participants: types.BitlistFromIndices([]uint64{1, 2, 3}), Proof: []byte{0x01}}
	if !coversParticipants(proof, types.BitlistFromIndices([]uint64{1, 3})) {
		t.Fatal("expected proof to cover requested participants")
	}
	if coversParticipants(proof, types.BitlistFromIndices([]uint64{1, 4})) {
		t.Fatal("proof reported missing participant as covered")
	}
}

func TestEngineUpdateHead(t *testing.T) {
	e := makeTestEngine()
	e.updateHead()

	head := e.Store.Head()
	if types.IsZeroRoot(head) {
		t.Fatal("head should not be zero after updateHead")
	}
}

// stateFinalizing builds an SSZ-serializable post-state pinned to the given
// finalized checkpoint, so it round-trips through the store like a real state.
func stateFinalizing(slot uint64, finalized *types.Checkpoint) *types.State {
	return &types.State{
		Config:                   &types.ChainConfig{GenesisTime: 1000},
		Slot:                     slot,
		LatestBlockHeader:        &types.BlockHeader{},
		LatestJustified:          finalized,
		LatestFinalized:          finalized,
		JustifiedSlots:           types.NewBitlistSSZ(0),
		JustificationsValidators: types.NewBitlistSSZ(0),
	}
}

func TestUpdateFinalizedFollowsCanonicalHead(t *testing.T) {
	e := makeTestEngine()
	genesis := e.Store.Head()

	var block1, block2 [32]byte
	block1[0] = 0x11
	block2[0] = 0x22

	e.Store.InsertBlockHeader(block1, &types.BlockHeader{Slot: 1, ParentRoot: genesis})
	e.Store.InsertBlockHeader(block2, &types.BlockHeader{Slot: 2, ParentRoot: block1})
	e.FC.OnBlock(1, block1, genesis)
	e.FC.OnBlock(2, block2, block1)

	// The head's post-state finalizes block1; finalization must re-anchor there.
	e.Store.InsertState(block2, stateFinalizing(2, &types.Checkpoint{Root: block1, Slot: 1}))
	e.Store.SetHead(block2)

	e.updateFinalizedFromHead(block2)

	got := e.Store.LatestFinalized()
	if got == nil || got.Slot != 1 || got.Root != block1 {
		t.Fatalf("finalized=%v, want slot 1 root 0x%x", got, block1)
	}
}

func TestDeriveFinalizedKeepsAnchorWhenNoBlockAtSlot(t *testing.T) {
	e := makeTestEngine()
	genesis := e.Store.Head()

	var block1 [32]byte
	block1[0] = 0x11
	e.Store.InsertBlockHeader(block1, &types.BlockHeader{Slot: 2, ParentRoot: genesis})
	// Post-state claims a finalized slot with no block on the chain at that slot
	// (e.g. a checkpoint-sync anchor); the trusted checkpoint must stand.
	e.Store.InsertState(block1, stateFinalizing(2, &types.Checkpoint{Root: block1, Slot: 1}))

	if cp := store.DeriveFinalizedFromHead(e.Store, block1); cp != nil {
		t.Fatalf("expected nil to keep anchor, got slot %d root 0x%x", cp.Slot, cp.Root)
	}
}

func TestUpdateFinalizedDoesNotLatchAboveHead(t *testing.T) {
	e := makeTestEngine()
	genesis := e.Store.Head()

	var block1, block2 [32]byte
	block1[0] = 0x11
	block2[0] = 0x22
	e.Store.InsertBlockHeader(block1, &types.BlockHeader{Slot: 1, ParentRoot: genesis})
	e.Store.InsertBlockHeader(block2, &types.BlockHeader{Slot: 2, ParentRoot: block1})
	e.FC.OnBlock(1, block1, genesis)
	e.FC.OnBlock(2, block2, block1)

	// A losing fork briefly latched finalization at slot 2; the canonical head's
	// post-state only finalizes block1 at slot 1, so finalization must move down
	// to track the head rather than stay latched above it.
	e.Store.SetLatestFinalized(&types.Checkpoint{Root: block2, Slot: 2})
	e.Store.InsertState(block2, stateFinalizing(2, &types.Checkpoint{Root: block1, Slot: 1}))
	e.Store.SetHead(block2)

	e.updateFinalizedFromHead(block2)

	got := e.Store.LatestFinalized()
	if got == nil || got.Slot != 1 || got.Root != block1 {
		t.Fatalf("finalized=%v, want slot 1 root 0x%x (must not latch above head)", got, block1)
	}
}

func TestEngineUpdateSafeTarget(t *testing.T) {
	e := makeTestEngine()
	e.updateSafeTarget()

	safeTarget := e.Store.SafeTarget()
	if types.IsZeroRoot(safeTarget) {
		t.Fatal("safe target should not be zero")
	}
}

func makeSafeTargetEngine(t *testing.T, numValidators int) (*Engine, [32]byte) {
	t.Helper()
	e := makeTestEngine()

	genesis := e.Store.Head()
	var block1, block2 [32]byte
	block1[0] = 0x11
	block2[0] = 0x22

	e.Store.InsertBlockHeader(block1, &types.BlockHeader{Slot: 1, ParentRoot: genesis})
	e.Store.InsertBlockHeader(block2, &types.BlockHeader{Slot: 2, ParentRoot: block1})
	e.FC.OnBlock(1, block1, genesis)
	e.FC.OnBlock(2, block2, block1)

	headState := e.Store.GetState(genesis)
	headState.Validators = make([]*types.Validator, numValidators)
	for i := range headState.Validators {
		headState.Validators[i] = &types.Validator{}
	}
	e.Store.InsertState(genesis, headState)

	return e, block2
}

func planAggregatedVoteForBlock(t *testing.T, targetRoot [32]byte, targetSlot, numValidators, numVoters uint64) ([32]byte, *types.AttestationData, *types.SingleMessageAggregate) {
	t.Helper()

	bits := types.NewBitlistSSZ(numValidators)
	for i := range numVoters {
		types.BitlistSet(bits, i)
	}
	data := &types.AttestationData{
		Slot:   targetSlot,
		Head:   &types.Checkpoint{Root: targetRoot, Slot: targetSlot},
		Target: &types.Checkpoint{Root: targetRoot, Slot: targetSlot},
		Source: &types.Checkpoint{},
	}
	dataRoot, err := data.HashTreeRoot()
	if err != nil {
		t.Fatalf("hash attestation data: %v", err)
	}
	return dataRoot, data, &types.SingleMessageAggregate{Participants: bits, Proof: []byte{0x01}}
}

func TestUpdateSafeTarget_IgnoresKnownPool(t *testing.T) {
	const numValidators = 6

	t.Run("known_pool_only_does_not_advance", func(t *testing.T) {
		e, block2 := makeSafeTargetEngine(t, numValidators)
		genesis := e.Store.Head()

		dataRoot, data, proof := planAggregatedVoteForBlock(t, block2, 2, numValidators, 4)
		e.Store.KnownPayloads.Push(dataRoot, data, proof)

		e.updateSafeTarget()

		if e.Store.SafeTarget() != genesis {
			t.Fatalf("safe target advanced past genesis from known-pool-only votes (root=0x%x); safe target must ignore known-pool-only votes",
				e.Store.SafeTarget())
		}
	})

	t.Run("new_pool_advances", func(t *testing.T) {
		e, block2 := makeSafeTargetEngine(t, numValidators)

		dataRoot, data, proof := planAggregatedVoteForBlock(t, block2, 2, numValidators, 4)
		e.Store.NewPayloads.Push(dataRoot, data, proof)

		e.updateSafeTarget()

		if e.Store.SafeTarget() != block2 {
			t.Fatalf("safe target did not advance to block_2 with 4-of-6 new-pool votes; got 0x%x",
				e.Store.SafeTarget())
		}
	})
}

func TestEnginePendingBlocks(t *testing.T) {
	e := makeTestEngine()

	var blockRoot, parentRoot [32]byte
	blockRoot[0] = 0x10
	parentRoot[0] = 0x20

	e.Pending.SetParent(blockRoot, parentRoot)
	e.Pending.AddChild(parentRoot, blockRoot)

	if e.Pending.ParentBuckets() != 1 {
		t.Fatalf("expected 1 pending parent, got %d", e.Pending.ParentBuckets())
	}
	if e.Pending.Entries() != 1 {
		t.Fatalf("expected 1 pending block, got %d", e.Pending.Entries())
	}
}

func TestEngineRun_InvokesInitialOnTick(t *testing.T) {
	e := makeTestEngine()

	if got := e.Store.Time(); got != 0 {
		t.Fatalf("precondition: expected store.time=0 at start, got %d", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		e.Run(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
			t.Error("Engine.Run did not exit after context cancel")
		}
	}()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if e.Store.Time() > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("Engine.Run did not invoke initial onTick within 200ms; store.time=%d", e.Store.Time())
}

func TestOnTickNilAggregationController(t *testing.T) {
	e := makeTestEngine()
	e.AggCtl = nil

	e.onTick()
}

func TestProcessOneBlock_RejectsPreFinalized(t *testing.T) {
	e := makeTestEngine()

	var finalizedRoot, parentRoot [32]byte
	finalizedRoot[0] = 0x05
	parentRoot[0] = 0xBB
	e.Store.SetLatestFinalized(&types.Checkpoint{Root: finalizedRoot, Slot: 10})

	signedBlock := &types.SignedBlock{
		Block: &types.Block{
			Slot:       5,
			ParentRoot: parentRoot,
			Body:       &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{},
	}

	var queue []*types.SignedBlock
	e.processOneBlock(signedBlock, &queue)

	if e.Pending.ParentBuckets() != 0 {
		t.Fatalf("pre-finalized block was buffered as pending; ParentBuckets=%d", e.Pending.ParentBuckets())
	}
	if e.Pending.Entries() != 0 {
		t.Fatalf("pre-finalized block recorded a missing-parent entry; Entries=%d", e.Pending.Entries())
	}
	if len(queue) != 0 {
		t.Fatalf("pre-finalized block produced cascade work; queue=%d", len(queue))
	}
}

func TestProcessOneBlock_AdmitsAtFinalizedSlot(t *testing.T) {
	e := makeTestEngine()

	var finalizedRoot, parentRoot [32]byte
	finalizedRoot[0] = 0x05
	parentRoot[0] = 0xBB
	e.Store.SetLatestFinalized(&types.Checkpoint{Root: finalizedRoot, Slot: 10})

	signedBlock := &types.SignedBlock{
		Block: &types.Block{
			Slot:       10,
			ParentRoot: parentRoot,
			Body:       &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{},
	}

	var queue []*types.SignedBlock
	e.processOneBlock(signedBlock, &queue)

	if e.Pending.Entries() == 0 {
		t.Fatal("block at finalized slot was rejected by the strict-less-than guard")
	}
}

func TestEngineCascadePending(t *testing.T) {
	e := makeTestEngine()

	var parentRoot, child1, child2 [32]byte
	parentRoot[0] = 0x01
	child1[0] = 0x10
	child2[0] = 0x20

	e.Pending.SetParent(child1, parentRoot)
	e.Pending.SetParent(child2, parentRoot)
	e.Pending.AddChild(parentRoot, child1)
	e.Pending.AddChild(parentRoot, child2)

	if e.Pending.ChildCount(parentRoot) != 2 {
		t.Fatalf("expected 2 children pending, got %d", e.Pending.ChildCount(parentRoot))
	}

	var queue []*types.SignedBlock
	e.collectPendingChildren(parentRoot, &queue)

	if e.Pending.ParentBuckets() != 0 {
		t.Fatalf("expected 0 pending after cascade, got %d", e.Pending.ParentBuckets())
	}
	if e.Pending.Entries() != 0 {
		t.Fatalf("expected 0 pending parents after cascade, got %d", e.Pending.Entries())
	}
}

func TestEngineMessageHandler(t *testing.T) {
	e := makeTestEngine()

	block := &types.SignedBlock{
		Block: &types.Block{Slot: 1},
		Proof: &types.MultiMessageAggregate{},
	}

	e.OnBlock(block)

	select {
	case received := <-e.BlockCh:
		if received.Block.Slot != 1 {
			t.Fatal("wrong block slot")
		}
	default:
		t.Fatal("block should be in channel")
	}
}

func TestEngineGetOurProposer(t *testing.T) {
	e := makeTestEngine()
	_, ok := e.getOurProposer(1)
	if ok {
		t.Fatal("should not be proposer without keys")
	}
}

func TestEngineCurrentSlot(t *testing.T) {
	e := makeTestEngine()
	slot := e.currentSlot(1004000)
	if slot != 1 {
		t.Fatalf("expected slot 1, got %d", slot)
	}
}

func TestEngineCurrentInterval(t *testing.T) {
	e := makeTestEngine()
	interval := e.currentInterval(1004800)
	if interval != 1 {
		t.Fatalf("expected interval 1, got %d", interval)
	}
}

func TestEngineClockHandlesOverflowGenesisTime(t *testing.T) {
	e := makeTestEngine()
	e.Store.SetConfig(&types.ChainConfig{GenesisTime: ^uint64(0)/1000 + 1})

	if slot := e.currentSlot(^uint64(0)); slot != 0 {
		t.Fatalf("overflow genesis currentSlot=%d, want 0", slot)
	}
	if interval := e.currentInterval(^uint64(0)); interval != 0 {
		t.Fatalf("overflow genesis currentInterval=%d, want 0", interval)
	}
}

func TestNilEngineClockReturnsZero(t *testing.T) {
	var e *Engine
	if slot := e.currentSlot(1); slot != 0 {
		t.Fatalf("nil engine currentSlot=%d, want 0", slot)
	}
	if interval := e.currentInterval(1); interval != 0 {
		t.Fatalf("nil engine currentInterval=%d, want 0", interval)
	}
}

func TestComputeSyncStatus_FreshNodePastWallClock(t *testing.T) {
	e := makeTestEngine()

	if status := e.computeSyncStatus(39); status != syncer.SyncSyncing {
		t.Errorf("head=0 currentSlot=39 should be syncer.SyncSyncing, got %s", status)
	}

	if status := e.computeSyncStatus(2); status != syncer.SyncSynced {
		t.Errorf("head=0 currentSlot=2 should be syncer.SyncSynced, got %s", status)
	}
}

func TestComputeSyncStatusAvoidsHeadSlotOverflow(t *testing.T) {
	e := makeTestEngine()
	head := e.Store.Head()
	e.Store.InsertBlockHeader(head, &types.BlockHeader{Slot: ^uint64(0)})

	if status := e.computeSyncStatus(^uint64(0)); status != syncer.SyncSynced {
		t.Fatalf("max head/current slot status=%s, want %s", status, syncer.SyncSynced)
	}
}

func TestCascadeClearsDepth(t *testing.T) {
	e := makeTestEngine()

	var parentRoot, child1 [32]byte
	parentRoot[0] = 0x01
	child1[0] = 0x10

	e.Pending.SetParent(child1, parentRoot)
	e.Pending.SetDepth(child1, 5)
	e.Pending.AddChild(parentRoot, child1)

	var queue []*types.SignedBlock
	e.collectPendingChildren(parentRoot, &queue)

	if _, ok := e.Pending.Depth(child1); ok {
		t.Fatal("depth should be cleared after collectPendingChildren")
	}
}

func TestBufferMissingParentKeepsImmediateParentLink(t *testing.T) {
	e := makeTestEngine()

	var missingRoot, parentRoot, blockRoot [32]byte
	missingRoot[0] = 0xAA
	parentRoot[0] = 0xBB
	blockRoot[0] = 0xCC

	e.Pending.SetParent(parentRoot, missingRoot)
	e.Pending.SetDepth(parentRoot, 1)
	e.Pending.AddChild(missingRoot, parentRoot)

	signedBlock := &types.SignedBlock{
		Block: &types.Block{
			Slot:       2,
			ParentRoot: parentRoot,
			Body:       &types.BlockBody{},
		},
		Proof: &types.MultiMessageAggregate{},
	}

	var queue []*types.SignedBlock
	e.bufferMissingParentBlock(signedBlock, blockRoot, parentRoot, &queue)

	if got := e.Pending.ResolveAncestor(blockRoot); got != missingRoot {
		t.Fatalf("resolved ancestor=0x%x, want missing root 0x%x", got, missingRoot)
	}
	if e.Pending.ChildCount(parentRoot) != 1 {
		t.Fatalf("parent child count=%d, want 1", e.Pending.ChildCount(parentRoot))
	}

	e.Pending.DiscardSubtree(blockRoot)
	if e.Pending.ChildCount(parentRoot) != 0 {
		t.Fatalf("stale child entry left under immediate parent; count=%d", e.Pending.ChildCount(parentRoot))
	}
	if e.Pending.Count() != 1 {
		t.Fatalf("pending count=%d, want only parentRoot waiting on missingRoot", e.Pending.Count())
	}
}
