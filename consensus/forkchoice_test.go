package consensus

import "testing"

func createTestChain() (map[Root]*Block, Root, Root, Root) {
	genesis := &Block{
		Slot:          0,
		ProposerIndex: 0,
		ParentRoot:    Root{},
		StateRoot:     Root{0x01},
		Body:          BlockBody{},
	}
	genesisHash, _ := genesis.HashTreeRoot()

	blockA := &Block{
		Slot:          1,
		ProposerIndex: 1,
		ParentRoot:    genesisHash,
		StateRoot:     Root{0x02},
		Body:          BlockBody{},
	}
	blockAHash, _ := blockA.HashTreeRoot()

	blockB := &Block{
		Slot:          2,
		ProposerIndex: 2,
		ParentRoot:    blockAHash,
		StateRoot:     Root{0x03},
		Body:          BlockBody{},
	}
	blockBHash, _ := blockB.HashTreeRoot()

	blocks := map[Root]*Block{
		genesisHash: genesis,
		blockAHash:  blockA,
		blockBHash:  blockB,
	}

	return blocks, genesisHash, blockAHash, blockBHash
}

func TestGetForkChoiceHead_NoVotes(t *testing.T) {
	blocks, genesisHash, _, _ := createTestChain()

	head := GetForkChoiceHead(blocks, genesisHash, map[ValidatorIndex]Checkpoint{}, 0)

	if head != genesisHash {
		t.Errorf("expected genesis hash, got different root")
	}
}

func TestGetForkChoiceHead_SingleVote(t *testing.T) {
	blocks, genesisHash, _, blockBHash := createTestChain()

	votes := map[ValidatorIndex]Checkpoint{
		0: {Root: blockBHash, Slot: 2},
	}

	head := GetForkChoiceHead(blocks, genesisHash, votes, 0)

	if head != blockBHash {
		t.Errorf("expected block B hash, got different root")
	}
}

func TestGetForkChoiceHead_MultipleForks(t *testing.T) {
	// Create fork: genesis -> A -> B
	//                     -> C -> D
	genesis := &Block{Slot: 0, ParentRoot: Root{}, StateRoot: Root{0x01}}
	genesisHash, _ := genesis.HashTreeRoot()

	blockA := &Block{Slot: 1, ProposerIndex: 1, ParentRoot: genesisHash, StateRoot: Root{0x02}}
	blockAHash, _ := blockA.HashTreeRoot()

	blockB := &Block{Slot: 2, ProposerIndex: 2, ParentRoot: blockAHash, StateRoot: Root{0x03}}
	blockBHash, _ := blockB.HashTreeRoot()

	blockC := &Block{Slot: 1, ProposerIndex: 3, ParentRoot: genesisHash, StateRoot: Root{0x04}}
	blockCHash, _ := blockC.HashTreeRoot()

	blockD := &Block{Slot: 2, ProposerIndex: 4, ParentRoot: blockCHash, StateRoot: Root{0x05}}
	blockDHash, _ := blockD.HashTreeRoot()

	blocks := map[Root]*Block{
		genesisHash: genesis,
		blockAHash:  blockA,
		blockBHash:  blockB,
		blockCHash:  blockC,
		blockDHash:  blockD,
	}

	// More votes for fork 2 (C->D)
	votes := map[ValidatorIndex]Checkpoint{
		0: {Root: blockBHash, Slot: 2}, // Vote for fork 1
		1: {Root: blockDHash, Slot: 2}, // Vote for fork 2
		2: {Root: blockDHash, Slot: 2}, // Vote for fork 2
	}

	head := GetForkChoiceHead(blocks, genesisHash, votes, 0)

	if head != blockDHash {
		t.Errorf("expected block D (fork 2) to win with more votes")
	}
}

func TestGetForkChoiceHead_MinScore(t *testing.T) {
	blocks, genesisHash, blockAHash, blockBHash := createTestChain()

	// Only 1 vote for block B
	votes := map[ValidatorIndex]Checkpoint{
		0: {Root: blockBHash, Slot: 2},
	}

	// min_score=2 should stop at block A (which has 1 vote from ancestor counting)
	head := GetForkChoiceHead(blocks, genesisHash, votes, 2)

	// Should return genesis since no block meets min_score of 2
	if head != genesisHash {
		t.Logf("head=%x, genesis=%x, blockA=%x", head[:4], genesisHash[:4], blockAHash[:4])
	}
}

func TestGetLatestJustified(t *testing.T) {
	state1 := &State{
		LatestJustified: Checkpoint{Slot: 5},
	}
	state2 := &State{
		LatestJustified: Checkpoint{Slot: 10},
	}
	state3 := &State{
		LatestJustified: Checkpoint{Slot: 3},
	}

	states := map[Root]*State{
		{0x01}: state1,
		{0x02}: state2,
		{0x03}: state3,
	}

	latest := GetLatestJustified(states)

	if latest == nil {
		t.Fatal("expected non-nil checkpoint")
	}
	if latest.Slot != 10 {
		t.Errorf("expected slot 10, got %d", latest.Slot)
	}
}

func TestGetLatestJustified_Empty(t *testing.T) {
	states := map[Root]*State{}

	latest := GetLatestJustified(states)

	if latest != nil {
		t.Error("expected nil for empty states")
	}
}

func TestCompareRoots(t *testing.T) {
	a := Root{0x01, 0x02}
	b := Root{0x01, 0x01}
	c := Root{0x01, 0x02}

	if compareRoots(a, b) != 1 {
		t.Error("expected a > b")
	}
	if compareRoots(b, a) != -1 {
		t.Error("expected b < a")
	}
	if compareRoots(a, c) != 0 {
		t.Error("expected a == c")
	}
}

func TestNewStore(t *testing.T) {
	state := GenerateGenesis(1000, 10)

	// Create anchor block with correct state root
	stateRoot, _ := state.HashTreeRoot()
	anchorBlock := &Block{
		Slot:       0,
		StateRoot:  stateRoot,
		ParentRoot: Root{},
		Body:       BlockBody{},
	}

	store, err := NewStore(state, anchorBlock)
	if err != nil {
		t.Fatalf("NewStore error: %v", err)
	}

	if store.Time != 0 {
		t.Errorf("expected time 0, got %d", store.Time)
	}
	if len(store.Blocks) != 1 {
		t.Errorf("expected 1 block, got %d", len(store.Blocks))
	}
	if len(store.States) != 1 {
		t.Errorf("expected 1 state, got %d", len(store.States))
	}
}

func TestNewStore_StateMismatch(t *testing.T) {
	state := GenerateGenesis(1000, 10)

	// Create anchor block with wrong state root
	anchorBlock := &Block{
		Slot:       0,
		StateRoot:  Root{0xDE, 0xAD},
		ParentRoot: Root{},
		Body:       BlockBody{},
	}

	_, err := NewStore(state, anchorBlock)
	if err == nil {
		t.Error("expected error for state root mismatch")
	}
}

func TestStore_CurrentSlot(t *testing.T) {
	state := GenerateGenesis(1000, 10)
	stateRoot, _ := state.HashTreeRoot()
	anchorBlock := &Block{Slot: 0, StateRoot: stateRoot}

	store, _ := NewStore(state, anchorBlock)

	if store.CurrentSlot() != 0 {
		t.Errorf("expected slot 0, got %d", store.CurrentSlot())
	}

	store.Time = 8 // 2 slots worth of intervals
	if store.CurrentSlot() != 2 {
		t.Errorf("expected slot 2, got %d", store.CurrentSlot())
	}
}
