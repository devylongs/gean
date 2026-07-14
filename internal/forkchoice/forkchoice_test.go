package forkchoice

import (
	"testing"

	"github.com/geanlabs/gean/internal/types"
)

func root(b byte) [32]byte {
	var r [32]byte
	r[0] = b
	return r
}

func rootsEqual(a, b [][32]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func makeAttData(headRoot [32]byte, slot uint64) *types.AttestationData {
	return &types.AttestationData{
		Slot:   slot,
		Head:   &types.Checkpoint{Root: headRoot, Slot: slot},
		Target: &types.Checkpoint{},
		Source: &types.Checkpoint{},
	}
}
func TestSpecComputeBlockWeights(t *testing.T) {
	// Chain: root_a (slot 0) -> root_b (slot 1) -> root_c (slot 2)
	rootA, rootB, rootC := root(1), root(2), root(3)
	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 2, ParentRoot: rootB},
	}
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootC, 2),
		1: makeAttData(rootB, 1),
	}

	weights := SpecComputeBlockWeights(0, blocks, attestations)

	// rootC: 1 vote (validator 0)
	if weights[rootC] != 1 {
		t.Fatalf("rootC weight: expected 1, got %d", weights[rootC])
	}
	// rootB: 2 votes (validator 0 walks through + validator 1 direct)
	if weights[rootB] != 2 {
		t.Fatalf("rootB weight: expected 2, got %d", weights[rootB])
	}
	// rootA: at slot 0 = start_slot, not counted
	if weights[rootA] != 0 {
		t.Fatalf("rootA weight: expected 0, got %d", weights[rootA])
	}
}

func TestSpecComputeBlockWeightsEmpty(t *testing.T) {
	weights := SpecComputeBlockWeights(0, nil, nil)
	if len(weights) != 0 {
		t.Fatal("expected empty weights")
	}
}

func TestSpecLMDGhostLinearChain(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)
	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 2, ParentRoot: rootB},
	}
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootC, 2),
	}

	head, _, _ := SpecComputeLMDGhostHead(rootA, blocks, attestations, 0)
	if head != rootC {
		t.Fatalf("expected rootC, got %x", head[:4])
	}
}

func TestSpecLMDGhostForkHeavier(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)
	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 1, ParentRoot: rootA},
	}
	// 2 votes for rootB, 1 for rootC -> rootB wins
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootB, 1),
		1: makeAttData(rootB, 1),
		2: makeAttData(rootC, 1),
	}

	head, _, _ := SpecComputeLMDGhostHead(rootA, blocks, attestations, 0)
	if head != rootB {
		t.Fatalf("expected rootB (heavier), got %x", head[:4])
	}
}

func TestSpecLMDGhostTiebreakLexicographic(t *testing.T) {
	rootA := root(1)
	rootB := root(2) // smaller
	rootC := root(3) // larger -> wins tiebreak
	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 1, ParentRoot: rootA},
	}
	// Equal weight: 1 vote each -> lexicographic tiebreak, rootC > rootB
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootB, 1),
		1: makeAttData(rootC, 1),
	}

	head, _, _ := SpecComputeLMDGhostHead(rootA, blocks, attestations, 0)
	if head != rootC {
		t.Fatalf("expected rootC (lexicographic tiebreak), got %x", head[:4])
	}
}
func TestProtoArrayLinearChain(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(2, rootC, rootB)

	// Validator 0 attests to rootC
	fc.votes.SetKnown(0, fc.NodeIndex(rootC), 2, makeAttData(rootC, 2))

	head := fc.UpdateHead(rootA)
	if head != rootC {
		t.Fatalf("expected rootC, got %x", head[:4])
	}
}

func TestProtoArrayForkHeavier(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(1, rootC, rootA)

	// 2 votes for rootB, 1 for rootC
	fc.votes.SetKnown(0, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	fc.votes.SetKnown(1, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	fc.votes.SetKnown(2, fc.NodeIndex(rootC), 1, makeAttData(rootC, 1))

	head := fc.UpdateHead(rootA)
	if head != rootB {
		t.Fatalf("expected rootB (heavier), got %x", head[:4])
	}
}

func TestProtoArrayTiebreakLexicographic(t *testing.T) {
	rootA := root(1)
	rootB := root(2)
	rootC := root(3) // larger -> wins
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(1, rootC, rootA)

	fc.votes.SetKnown(0, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	fc.votes.SetKnown(1, fc.NodeIndex(rootC), 1, makeAttData(rootC, 1))

	head := fc.UpdateHead(rootA)
	if head != rootC {
		t.Fatalf("expected rootC (tiebreak), got %x", head[:4])
	}
}

func TestProtoArrayNoAttestations(t *testing.T) {
	rootA := root(1)
	fc := New(0, rootA, [32]byte{})
	head := fc.UpdateHead(rootA)
	if head != rootA {
		t.Fatalf("expected rootA with no attestations, got %x", head[:4])
	}
}

func TestProtoArrayReparentsOrphanWhenParentArrives(t *testing.T) {
	anchor, parent, child := root(1), root(2), root(3)
	fc := New(0, anchor, [32]byte{})

	fc.OnBlock(2, child, parent)
	childIdx := fc.NodeIndex(child)
	if childIdx < 0 {
		t.Fatal("orphan child should be indexed")
	}
	if fc.array.nodes[childIdx].Parent != -1 {
		t.Fatalf("orphan child parent index=%d, want -1", fc.array.nodes[childIdx].Parent)
	}

	fc.OnBlock(1, parent, anchor)
	childIdx = fc.NodeIndex(child)
	parentIdx := fc.NodeIndex(parent)
	if childIdx < 0 || parentIdx < 0 {
		t.Fatal("parent and child should both be indexed")
	}
	if fc.array.nodes[childIdx].Parent != parentIdx {
		t.Fatalf("child parent index=%d, want %d", fc.array.nodes[childIdx].Parent, parentIdx)
	}

	fc.votes.SetKnown(0, childIdx, 2, makeAttData(child, 2))
	if head := fc.UpdateHead(anchor); head != child {
		t.Fatalf("head=%x, want reparented child %x", head[:4], child[:4])
	}
}

func TestCanonicalAnalysisIncludesReparentedOrphanDescendant(t *testing.T) {
	anchor, parent, child, fork := root(1), root(2), root(3), root(4)
	fc := New(0, anchor, [32]byte{})
	fc.OnBlock(2, child, parent)
	fc.OnBlock(1, parent, anchor)
	fc.OnBlock(1, fork, anchor)

	canonical, nonCanonical := fc.GetCanonicalAnalysis(parent)
	if !rootsEqual(canonical, [][32]byte{parent, anchor}) {
		t.Fatalf("canonical=%x, want parent,anchor", canonical)
	}
	if !rootsEqual(nonCanonical, [][32]byte{fork}) {
		t.Fatalf("nonCanonical=%x, want fork only", nonCanonical)
	}
}

func TestProtoArrayRejectsInvalidParentSlot(t *testing.T) {
	anchor, child := root(1), root(2)
	fc := New(2, anchor, [32]byte{})

	fc.OnBlock(2, child, anchor)
	if idx := fc.NodeIndex(child); idx != -1 {
		t.Fatalf("invalid child index=%d, want -1", idx)
	}

	fc.OnBlock(3, child, child)
	if idx := fc.NodeIndex(child); idx != -1 {
		t.Fatalf("self-parented child index=%d, want -1", idx)
	}
}

func TestProtoArrayPartialValueGuards(t *testing.T) {
	var nilArray *ProtoArray
	if nilArray.Len() != 0 {
		t.Fatal("nil protoarray length should be 0")
	}
	if nilArray.Nodes() != nil {
		t.Fatal("nil protoarray nodes should be nil")
	}
	nilArray.OnBlock(1, root(2), root(1))
	nilArray.ApplyScoreChanges([]int64{1}, 0)
	if got := nilArray.FindHead(root(1)); got != root(1) {
		t.Fatalf("nil protoarray head=%x, want justified root", got[:4])
	}
	nilArray.Prune(root(1))

	var zero ProtoArray
	anchor := root(1)
	zero.OnBlock(0, anchor, [32]byte{})
	if zero.Len() != 1 {
		t.Fatalf("zero protoarray len=%d, want 1", zero.Len())
	}
	if got := zero.FindHead(anchor); got != anchor {
		t.Fatalf("zero protoarray head=%x, want anchor", got[:4])
	}
}

func TestProtoArrayDoesNotReparentInvalidOrphanSlot(t *testing.T) {
	anchor, parent, child := root(1), root(2), root(3)
	fc := New(0, anchor, [32]byte{})

	fc.OnBlock(1, child, parent)
	fc.OnBlock(2, parent, anchor)

	childIdx := fc.NodeIndex(child)
	if childIdx < 0 {
		t.Fatal("orphan child should remain indexed")
	}
	if fc.array.nodes[childIdx].Parent != -1 {
		t.Fatalf("invalid orphan parent index=%d, want -1", fc.array.nodes[childIdx].Parent)
	}
}

func TestPruneReparentedOrphanKeepsDescendantAndRemapsVote(t *testing.T) {
	anchor, parent, child := root(1), root(2), root(3)
	fc := New(0, anchor, [32]byte{})
	fc.OnBlock(2, child, parent)
	fc.OnBlock(1, parent, anchor)

	childIdx := fc.NodeIndex(child)
	fc.votes.SetKnown(0, childIdx, 2, makeAttData(child, 2))
	if head := fc.UpdateHead(anchor); head != child {
		t.Fatalf("head before prune=%x, want child", head[:4])
	}

	fc.Prune(parent)
	parentIdx := fc.NodeIndex(parent)
	childIdx = fc.NodeIndex(child)
	if parentIdx != 0 || childIdx != 1 {
		t.Fatalf("post-prune parent/child indexes=%d/%d, want 0/1", parentIdx, childIdx)
	}
	if fc.NodeIndex(anchor) != -1 {
		t.Fatal("anchor should be pruned")
	}
	tracker := fc.votes.Votes[0]
	if tracker.AppliedIndex != childIdx {
		t.Fatalf("applied index=%d, want child index %d", tracker.AppliedIndex, childIdx)
	}
	if tracker.LatestKnown == nil || tracker.LatestKnown.Index != childIdx {
		t.Fatalf("latest known=%+v, want child index %d", tracker.LatestKnown, childIdx)
	}
	if head := fc.UpdateHead(parent); head != child {
		t.Fatalf("head after prune=%x, want child", head[:4])
	}
}

func TestUpdateSafeTargetDoesNotReuseHeadDescendant(t *testing.T) {
	rootA, rootB := root(1), root(2)
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)

	fc.votes.SetKnown(0, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	if head := fc.UpdateHead(rootA); head != rootB {
		t.Fatalf("head=%x, want rootB", head[:4])
	}

	if safe := fc.UpdateSafeTarget(rootA, 3); safe != rootA {
		t.Fatalf("safe target reused head descendant: got %x, want rootA", safe[:4])
	}
}

func TestUpdateSafeTargetUsesNewVotesAboveThreshold(t *testing.T) {
	rootA, rootB := root(1), root(2)
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)

	for vid := uint64(0); vid < 3; vid++ {
		fc.votes.SetNew(vid, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	}

	if safe := fc.UpdateSafeTarget(rootA, 4); safe != rootB {
		t.Fatalf("safe target=%x, want rootB", safe[:4])
	}
}

func TestUpdateSafeTargetZeroValidatorsStaysAtJustifiedRoot(t *testing.T) {
	rootA, rootB := root(1), root(2)
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)

	if safe := fc.UpdateSafeTarget(rootA, 0); safe != rootA {
		t.Fatalf("safe target=%x, want justified root %x", safe[:4], rootA[:4])
	}
}

func TestProtoArrayVoteChange(t *testing.T) {
	rootA, rootB, rootC, rootD := root(1), root(2), root(3), root(4)
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(1, rootC, rootA)
	fc.OnBlock(2, rootD, rootC)

	// Initially vote for rootB at slot 1.
	fc.votes.SetKnown(0, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	head := fc.UpdateHead(rootA)
	if head != rootB {
		t.Fatalf("expected rootB initially, got %x", head[:4])
	}

	// A later-slot re-vote is a normal target update and moves the head. The
	// latest vote per validator is last-wins, so the new target replaces the
	// old one.
	fc.votes.SetKnown(0, fc.NodeIndex(rootD), 2, makeAttData(rootD, 2))
	head = fc.UpdateHead(rootA)
	if head != rootD {
		t.Fatalf("expected rootD after vote change, got %x", head[:4])
	}
}

func TestProtoArrayPrune(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(2, rootC, rootB)

	if fc.array.Len() != 3 {
		t.Fatalf("expected 3 nodes, got %d", fc.array.Len())
	}

	fc.Prune(rootB)

	if fc.array.Len() != 2 {
		t.Fatalf("expected 2 nodes after prune, got %d", fc.array.Len())
	}
	if fc.NodeIndex(rootA) != -1 {
		t.Fatal("rootA should be pruned")
	}
	if fc.NodeIndex(rootB) < 0 {
		t.Fatal("rootB should still exist")
	}
}

// buildLinearChain inserts a linear chain (node k = root(k+1) at slot k, parent
// root(k), anchored at root(1)). reverse=true inserts children before parents, so
// orphan linking leaves interior nodes with a parent at a higher array index — the
// reverse-topological ordering that made the old fixed-point Prune O(N^2).
func buildLinearChain(depth int, reverse bool) *ForkChoice {
	fc := New(0, root(1), [32]byte{})
	insertK := func(k int) {
		parent := root(1)
		if k > 1 {
			parent = root(byte(k))
		}
		fc.OnBlock(uint64(k), root(byte(k+1)), parent)
	}
	if reverse {
		for k := depth; k >= 1; k-- {
			insertK(k)
		}
		return fc
	}
	for k := 1; k <= depth; k++ {
		insertK(k)
	}
	return fc
}

func countParentBackEdges(pa *ProtoArray) int {
	n := 0
	for i, node := range pa.nodes {
		if node.Parent > i {
			n++
		}
	}
	return n
}

// Prune must depend only on the tree, not on block arrival order: a chain built
// child-first (heavy parent back-edges) must prune to the exact same layout as the
// same chain built parent-first.
func TestPruneIndependentOfArrivalOrder(t *testing.T) {
	const depth = 10
	const finalizedAt = 4 // keep nodes 4..10, i.e. root(5)..root(11)
	finalizedRoot := root(byte(finalizedAt + 1))

	forward := buildLinearChain(depth, false)
	reverse := buildLinearChain(depth, true)

	// The two orderings must genuinely differ, or the test proves nothing.
	if got := countParentBackEdges(forward.array); got != 0 {
		t.Fatalf("forward chain should be topologically ordered, got %d back-edges", got)
	}
	if countParentBackEdges(reverse.array) == 0 {
		t.Fatal("reverse chain should have parent back-edges to exercise the O(N^2) path")
	}

	forward.Prune(finalizedRoot)
	reverse.Prune(finalizedRoot)

	fwd, rev := forward.array.Nodes(), reverse.array.Nodes()
	if len(fwd) != depth-finalizedAt+1 {
		t.Fatalf("kept %d nodes, want %d", len(fwd), depth-finalizedAt+1)
	}
	if len(fwd) != len(rev) {
		t.Fatalf("forward kept %d nodes, reverse kept %d", len(fwd), len(rev))
	}
	for i := range fwd {
		if fwd[i].Root != rev[i].Root || fwd[i].Parent != rev[i].Parent {
			t.Fatalf("node %d differs: forward{root=%x parent=%d} reverse{root=%x parent=%d}",
				i, fwd[i].Root[:1], fwd[i].Parent, rev[i].Root[:1], rev[i].Parent)
		}
	}

	// The finalized root anchors the pruned array; pre-finalized nodes are gone.
	if idx := forward.NodeIndex(finalizedRoot); idx != 0 {
		t.Fatalf("finalized root index=%d, want 0", idx)
	}
	if forward.NodeIndex(root(byte(finalizedAt))) != -1 {
		t.Fatal("pre-finalized node should be pruned")
	}
}

func TestCanonicalAnalysisSeparatesFinalizedForks(t *testing.T) {
	anchor, a, b, c, forkBefore, forkAfter := root(1), root(2), root(3), root(4), root(5), root(6)
	fc := New(0, anchor, [32]byte{})
	fc.OnBlock(1, a, anchor)
	fc.OnBlock(2, b, a)
	fc.OnBlock(3, c, b)
	fc.OnBlock(2, forkBefore, a)
	fc.OnBlock(3, forkAfter, b)

	canonical, nonCanonical := fc.GetCanonicalAnalysis(b)
	if !rootsEqual(canonical, [][32]byte{b, a, anchor}) {
		t.Fatalf("canonical=%x, want b,a,anchor", canonical)
	}
	if !rootsEqual(nonCanonical, [][32]byte{forkBefore}) {
		t.Fatalf("nonCanonical=%x, want forkBefore", nonCanonical)
	}
}

func TestProtoArrayDeepChain(t *testing.T) {
	roots := make([][32]byte, 10)
	for i := range roots {
		roots[i] = root(byte(i + 1))
	}
	fc := New(0, roots[0], [32]byte{})
	for i := 1; i < 10; i++ {
		fc.OnBlock(uint64(i), roots[i], roots[i-1])
	}

	// Attest to tip
	fc.votes.SetKnown(0, fc.NodeIndex(roots[9]), 9, makeAttData(roots[9], 9))
	head := fc.UpdateHead(roots[0])
	if head != roots[9] {
		t.Fatalf("expected root[9], got %x", head[:4])
	}
}
func TestSpecOracleLinearChain(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)

	// Spec
	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 2, ParentRoot: rootB},
	}
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootC, 2),
		1: makeAttData(rootB, 1),
	}
	specHead, _, _ := SpecComputeLMDGhostHead(rootA, blocks, attestations, 0)

	// Proto-array
	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(2, rootC, rootB)
	fc.votes.SetKnown(0, fc.NodeIndex(rootC), 2, makeAttData(rootC, 2))
	fc.votes.SetKnown(1, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	protoHead := fc.UpdateHead(rootA)

	if specHead != protoHead {
		t.Fatalf("oracle mismatch: spec=%x proto=%x", specHead[:4], protoHead[:4])
	}
}

func TestSpecOracleFork(t *testing.T) {
	rootA, rootB, rootC := root(1), root(2), root(3)

	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 1, ParentRoot: rootA},
	}
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootB, 1),
		1: makeAttData(rootB, 1),
		2: makeAttData(rootC, 1),
	}
	specHead, _, _ := SpecComputeLMDGhostHead(rootA, blocks, attestations, 0)

	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(1, rootC, rootA)
	fc.votes.SetKnown(0, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	fc.votes.SetKnown(1, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	fc.votes.SetKnown(2, fc.NodeIndex(rootC), 1, makeAttData(rootC, 1))
	protoHead := fc.UpdateHead(rootA)

	if specHead != protoHead {
		t.Fatalf("oracle mismatch: spec=%x proto=%x", specHead[:4], protoHead[:4])
	}
}

func TestSpecOracleTiebreak(t *testing.T) {
	rootA := root(1)
	rootB := root(2)
	rootC := root(3)

	blocks := map[[32]byte]BlockInfo{
		rootA: {Slot: 0, ParentRoot: [32]byte{}},
		rootB: {Slot: 1, ParentRoot: rootA},
		rootC: {Slot: 1, ParentRoot: rootA},
	}
	attestations := map[uint64]*types.AttestationData{
		0: makeAttData(rootB, 1),
		1: makeAttData(rootC, 1),
	}
	specHead, _, _ := SpecComputeLMDGhostHead(rootA, blocks, attestations, 0)

	fc := New(0, rootA, [32]byte{})
	fc.OnBlock(1, rootB, rootA)
	fc.OnBlock(1, rootC, rootA)
	fc.votes.SetKnown(0, fc.NodeIndex(rootB), 1, makeAttData(rootB, 1))
	fc.votes.SetKnown(1, fc.NodeIndex(rootC), 1, makeAttData(rootC, 1))
	protoHead := fc.UpdateHead(rootA)

	if specHead != protoHead {
		t.Fatalf("oracle mismatch: spec=%x proto=%x", specHead[:4], protoHead[:4])
	}
}

func TestReorgDepth(t *testing.T) {
	// Anchor (slot 0) -> A (slot 1) -> A2 (slot 2) -> A3 (slot 3)
	//                  \-> B (slot 1) -> B2 (slot 2)
	anchor, a, a2, a3, b, b2 := root(1), root(2), root(3), root(4), root(5), root(6)
	fc := New(0, anchor, [32]byte{})
	fc.OnBlock(1, a, anchor)
	fc.OnBlock(2, a2, a)
	fc.OnBlock(3, a3, a2)
	fc.OnBlock(1, b, anchor)
	fc.OnBlock(2, b2, b)

	cases := []struct {
		name      string
		oldHead   [32]byte
		newHead   [32]byte
		wantDepth uint64
	}{
		{"same head no reorg", a3, a3, 0},
		{"normal extension a to a2", a, a2, 0},
		{"1-block reorg a to b at slot 1", a, b, 1},
		{"2-block reorg a2 to b2 at slot 2", a2, b2, 2},
		{"3-block reorg a3 to b at slot 1", a3, b, 3},
		{"unknown old root", root(99), a, 0},
		{"unknown new root", a, root(99), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fc.ReorgDepth(tc.oldHead, tc.newHead)
			if got != tc.wantDepth {
				t.Fatalf("ReorgDepth(old=%x, new=%x) = %d, want %d",
					tc.oldHead[:1], tc.newHead[:1], got, tc.wantDepth)
			}
		})
	}
}

func TestAncestorAtDepth(t *testing.T) {
	anchor, a, b, c := root(1), root(2), root(3), root(4)
	fc := New(0, anchor, [32]byte{})
	fc.OnBlock(1, a, anchor)
	fc.OnBlock(2, b, a)
	fc.OnBlock(3, c, b)

	cases := []struct {
		depth    int
		wantRoot [32]byte
		wantSlot uint64
	}{
		{depth: 0, wantRoot: c, wantSlot: 3},
		{depth: 1, wantRoot: b, wantSlot: 2},
		{depth: 2, wantRoot: a, wantSlot: 1},
		{depth: 3, wantRoot: anchor, wantSlot: 0},
		{depth: 99, wantRoot: anchor, wantSlot: 0},
	}

	for _, tc := range cases {
		gotRoot, gotSlot, ok := fc.AncestorAtDepth(c, tc.depth)
		if !ok {
			t.Fatalf("depth %d returned ok=false", tc.depth)
		}
		if gotRoot != tc.wantRoot || gotSlot != tc.wantSlot {
			t.Fatalf("depth %d root/slot=%x/%d, want %x/%d",
				tc.depth, gotRoot[:4], gotSlot, tc.wantRoot[:4], tc.wantSlot)
		}
	}
}

func TestAncestorAtDepthStartsFromProvidedRoot(t *testing.T) {
	anchor, a, b, fork := root(1), root(2), root(3), root(4)
	fc := New(0, anchor, [32]byte{})
	fc.OnBlock(1, a, anchor)
	fc.OnBlock(2, b, a)
	fc.OnBlock(3, fork, anchor)

	gotRoot, gotSlot, ok := fc.AncestorAtDepth(b, 1)
	if !ok {
		t.Fatal("expected ancestor lookup to succeed")
	}
	if gotRoot != a || gotSlot != 1 {
		t.Fatalf("ancestor root/slot=%x/%d, want %x/1", gotRoot[:4], gotSlot, a[:4])
	}

	if _, _, ok := fc.AncestorAtDepth(root(99), 1); ok {
		t.Fatal("unknown root should return ok=false")
	}
}

func TestAnchorParentRootPreserved(t *testing.T) {
	anchorRoot := root(7)
	anchorParent := root(6)

	fc := New(12, anchorRoot, anchorParent)

	nodes := fc.array.Nodes()
	if len(nodes) != 1 {
		t.Fatalf("expected 1 anchor node, got %d", len(nodes))
	}
	if nodes[0].ParentRoot != anchorParent {
		t.Fatalf("anchor ParentRoot = %x, want %x", nodes[0].ParentRoot, anchorParent)
	}
	if nodes[0].Root != anchorRoot {
		t.Fatalf("anchor Root = %x, want %x", nodes[0].Root, anchorRoot)
	}
	if nodes[0].Slot != 12 {
		t.Fatalf("anchor Slot = %d, want 12", nodes[0].Slot)
	}
}

func TestForkChoicePublicAccessors(t *testing.T) {
	var nilFC *ForkChoice
	unknown := root(9)
	if nilFC.Len() != 0 {
		t.Fatal("nil forkchoice length should be 0")
	}
	if nilFC.Nodes() != nil {
		t.Fatal("nil forkchoice nodes should be nil")
	}
	if nilFC.NodeIndex(root(1)) != -1 {
		t.Fatal("nil forkchoice node lookup should fail")
	}
	if nilFC.SetKnownVote(0, root(1), 1, nil) {
		t.Fatal("nil forkchoice should reject known vote")
	}
	if nilFC.SetNewVote(0, root(1), 1, nil) {
		t.Fatal("nil forkchoice should reject new vote")
	}
	nilFC.OnBlock(1, root(2), root(1))
	if got := nilFC.UpdateHead(unknown); got != unknown {
		t.Fatalf("nil forkchoice head=%x, want justified root %x", got[:4], unknown[:4])
	}
	if got := nilFC.UpdateSafeTarget(unknown, 1); got != unknown {
		t.Fatalf("nil forkchoice safe target=%x, want justified root %x", got[:4], unknown[:4])
	}
	nilFC.Prune(root(1))
	if canonical, nonCanonical := nilFC.GetCanonicalAnalysis(root(1)); canonical != nil || nonCanonical != nil {
		t.Fatalf("nil forkchoice analysis=%v/%v, want nil/nil", canonical, nonCanonical)
	}
	if depth := nilFC.ReorgDepth(root(1), root(2)); depth != 0 {
		t.Fatalf("nil forkchoice reorg depth=%d, want 0", depth)
	}
	if _, _, ok := nilFC.AncestorAtDepth(root(1), 1); ok {
		t.Fatal("nil forkchoice ancestor lookup should fail")
	}
	if _, ok := nilFC.VoteTracker(0); ok {
		t.Fatal("nil forkchoice tracker lookup should fail")
	}

	anchor, child := root(1), root(2)
	fc := New(0, anchor, [32]byte{})
	fc.OnBlock(1, child, anchor)

	if fc.Len() != 2 {
		t.Fatalf("len=%d, want 2", fc.Len())
	}
	nodes := fc.Nodes()
	nodes[0].Weight = 99
	if fc.Nodes()[0].Weight == 99 {
		t.Fatal("Nodes should return a copy")
	}

	if !fc.SetKnownVote(0, child, 1, makeAttData(child, 1)) {
		t.Fatal("known vote for existing root should be accepted")
	}
	if !fc.SetNewVote(1, child, 1, makeAttData(child, 1)) {
		t.Fatal("new vote for existing root should be accepted")
	}
	if fc.SetKnownVote(2, root(99), 1, nil) {
		t.Fatal("known vote for unknown root should be rejected")
	}
	if fc.SetNewVote(3, root(99), 1, nil) {
		t.Fatal("new vote for unknown root should be rejected")
	}

	tracker, ok := fc.VoteTracker(0)
	if !ok || tracker.LatestKnown == nil {
		t.Fatalf("known tracker missing: %+v", tracker)
	}
	tracker.LatestKnown.Index = 99
	tracker.LatestKnown.Data.Head.Slot = 99
	tracker, ok = fc.VoteTracker(0)
	if !ok || tracker.LatestKnown.Index == 99 {
		t.Fatal("VoteTracker should return a copy of the known target")
	}
	if tracker.LatestKnown.Data.Head.Slot == 99 {
		t.Fatal("VoteTracker should copy nested attestation data")
	}

	tracker, ok = fc.VoteTracker(1)
	if !ok || tracker.LatestNew == nil {
		t.Fatalf("new tracker missing: %+v", tracker)
	}
	tracker.LatestNew.Index = 99
	tracker, ok = fc.VoteTracker(1)
	if !ok || tracker.LatestNew.Index == 99 {
		t.Fatal("VoteTracker should return a copy of the new target")
	}
}

func TestForkChoicePartialValueGuards(t *testing.T) {
	anchor, child := root(1), root(2)
	fc := &ForkChoice{array: NewProtoArray(0, anchor, [32]byte{})}
	fc.OnBlock(1, child, anchor)

	if fc.Len() != 2 {
		t.Fatalf("partial forkchoice len=%d, want 2", fc.Len())
	}
	if fc.SetKnownVote(0, child, 1, makeAttData(child, 1)) {
		t.Fatal("partial forkchoice without vote store should reject known vote")
	}
	if fc.SetNewVote(0, child, 1, makeAttData(child, 1)) {
		t.Fatal("partial forkchoice without vote store should reject new vote")
	}
	if head := fc.UpdateHead(anchor); head != child {
		t.Fatalf("partial forkchoice head=%x, want child", head[:4])
	}
	fc.Prune(child)
	if fc.NodeIndex(anchor) != -1 || fc.NodeIndex(child) != 0 {
		t.Fatal("partial forkchoice should prune protoarray even without vote store")
	}
}

func TestGenesisAnchorParentRootZero(t *testing.T) {
	genesisRoot := root(1)

	fc := New(0, genesisRoot, [32]byte{})

	nodes := fc.array.Nodes()
	if nodes[0].ParentRoot != ([32]byte{}) {
		t.Fatalf("genesis ParentRoot = %x, want zero", nodes[0].ParentRoot)
	}
}
