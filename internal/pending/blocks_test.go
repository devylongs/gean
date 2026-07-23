package pending

import "testing"

func TestBlockBuffer_Count(t *testing.T) {
	b := NewBlockBuffer()

	if b.Count() != 0 {
		t.Fatal("expected 0 pending blocks initially")
	}

	parent1 := [32]byte{0x01}
	parent2 := [32]byte{0x02}
	child1 := [32]byte{0x10}
	child2 := [32]byte{0x20}
	child3 := [32]byte{0x30}

	b.AddChild(parent1, child1)
	b.AddChild(parent1, child2)
	b.AddChild(parent2, child3)

	if b.Count() != 3 {
		t.Fatalf("expected 3 pending blocks, got %d", b.Count())
	}
	if b.ParentBuckets() != 2 {
		t.Fatalf("expected 2 parent buckets, got %d", b.ParentBuckets())
	}
	if b.ChildCount(parent1) != 2 {
		t.Fatalf("expected 2 children under parent1, got %d", b.ChildCount(parent1))
	}
}

func TestBlockBuffer_DepthTracking(t *testing.T) {
	b := NewBlockBuffer()

	root1 := [32]byte{0x01}
	root2 := [32]byte{0x02}
	root3 := [32]byte{0x03}

	b.SetDepth(root1, 1)
	b.SetDepth(root2, 2)
	b.SetDepth(root3, 3)

	if d, ok := b.Depth(root3); !ok || d != 3 {
		t.Fatalf("expected depth 3, got %d (ok=%v)", d, ok)
	}

	parentDepth, _ := b.Depth(root2)
	if parentDepth+1 != 3 {
		t.Fatalf("expected inherited depth 3, got %d", parentDepth+1)
	}

	b.ClearDepth(root3)
	if _, ok := b.Depth(root3); ok {
		t.Fatal("depth should be cleared")
	}
}

func TestBlockBuffer_ResolveAncestor(t *testing.T) {
	b := NewBlockBuffer()
	root := [32]byte{0x01}
	child := [32]byte{0x10}
	grandchild := [32]byte{0xA0}

	b.SetParent(grandchild, child)
	b.SetParent(child, root)

	if got := b.ResolveAncestor(grandchild); got != root {
		t.Fatalf("expected ancestor=root, got 0x%x", got)
	}
	if got := b.ResolveAncestor(root); got != root {
		t.Fatalf("unrecorded root should resolve to itself, got 0x%x", got)
	}
}

func TestBlockBuffer_ResolveAncestorCycle(t *testing.T) {
	b := NewBlockBuffer()
	a := [32]byte{0x01}
	bb := [32]byte{0x02}

	b.SetParent(a, bb)
	b.SetParent(bb, a)

	got := b.ResolveAncestor(a)
	if got != a {
		t.Fatalf("cycle should stop at repeated root: got 0x%x, want 0x%x", got, a)
	}
}

func TestBlockBuffer_DiscardSubtree(t *testing.T) {
	b := NewBlockBuffer()

	root := [32]byte{0x01}
	child1 := [32]byte{0x10}
	grandchild1 := [32]byte{0xA0}
	grandchild2 := [32]byte{0xB0}

	b.AddChild(root, child1)
	b.AddChild(child1, grandchild1)
	b.AddChild(child1, grandchild2)
	b.SetParent(child1, root)
	b.SetParent(grandchild1, child1)
	b.SetParent(grandchild2, child1)
	b.SetDepth(child1, 1)
	b.SetDepth(grandchild1, 2)
	b.SetDepth(grandchild2, 2)

	b.DiscardSubtree(child1)

	for _, r := range [][32]byte{child1, grandchild1, grandchild2} {
		if _, ok := b.parents[r]; ok {
			t.Fatalf("0x%x should be removed from parents", r)
		}
		if _, ok := b.depths[r]; ok {
			t.Fatalf("0x%x should be removed from depths", r)
		}
	}
	if _, ok := b.children[child1]; ok {
		t.Fatal("child1's child bucket should be removed")
	}
	if _, ok := b.children[root]; ok {
		t.Fatal("root's child bucket should be removed after child subtree discard")
	}
}

func TestBlockBuffer_DiscardSubtreeKeepsSiblings(t *testing.T) {
	b := NewBlockBuffer()

	root := [32]byte{0x01}
	child1 := [32]byte{0x10}
	child2 := [32]byte{0x20}

	b.AddChild(root, child1)
	b.AddChild(root, child2)
	b.SetParent(child1, root)
	b.SetParent(child2, root)

	b.DiscardSubtree(child1)

	if b.ChildCount(root) != 1 {
		t.Fatalf("root child count=%d, want 1", b.ChildCount(root))
	}
	if _, ok := b.children[root][child2]; !ok {
		t.Fatal("sibling child should remain")
	}
	if _, ok := b.parents[child1]; ok {
		t.Fatal("discarded child parent entry should be removed")
	}
}

func TestBlockBuffer_ZeroValueAndNilGuards(t *testing.T) {
	var zero BlockBuffer
	parent := [32]byte{0x01}
	child := [32]byte{0x02}

	zero.AddChild(parent, child)
	zero.SetParent(child, parent)
	zero.SetDepth(child, 3)
	if zero.Count() != 1 || zero.ParentBuckets() != 1 || zero.Entries() != 1 {
		t.Fatalf("zero-value counts=%d/%d/%d, want 1/1/1",
			zero.Count(), zero.ParentBuckets(), zero.Entries())
	}
	if depth, ok := zero.Depth(child); !ok || depth != 3 {
		t.Fatalf("zero-value depth=%d ok=%v, want 3/true", depth, ok)
	}
	if got := zero.ResolveAncestor(child); got != parent {
		t.Fatalf("zero-value ancestor=0x%x, want parent 0x%x", got, parent)
	}
	zero.DiscardSubtree(child)
	if zero.Count() != 0 || zero.Entries() != 0 {
		t.Fatalf("zero-value after discard counts=%d/%d, want 0/0", zero.Count(), zero.Entries())
	}

	var nilBuffer *BlockBuffer
	nilBuffer.AddChild(parent, child)
	nilBuffer.SetParent(child, parent)
	nilBuffer.SetDepth(child, 3)
	nilBuffer.ClearDepth(child)
	nilBuffer.ClearEntry(child)
	nilBuffer.RemoveChild(parent, child)
	nilBuffer.DiscardSubtree(child)
	if set, ok := nilBuffer.RemoveBucket(parent); ok || set != nil {
		t.Fatalf("nil RemoveBucket=%v/%t, want nil/false", set, ok)
	}
	if got := nilBuffer.Count(); got != 0 {
		t.Fatalf("nil Count=%d, want 0", got)
	}
	if got := nilBuffer.ResolveAncestor(child); got != child {
		t.Fatalf("nil ancestor=0x%x, want start 0x%x", got, child)
	}
	if pairs := nilBuffer.Pairs(); pairs != nil {
		t.Fatalf("nil pairs=%v, want nil", pairs)
	}
}

func TestBlockBuffer_HighestSlotEntry(t *testing.T) {
	b := NewBlockBuffer()

	if _, _, ok := b.HighestSlotEntry(); ok {
		t.Fatal("expected no entry on empty buffer")
	}

	near := [32]byte{0x01}
	mid := [32]byte{0x02}
	far := [32]byte{0x03}
	b.SetSlot(near, 10)
	b.SetSlot(mid, 50)
	b.SetSlot(far, 90)

	root, slot, ok := b.HighestSlotEntry()
	if !ok || root != far || slot != 90 {
		t.Fatalf("expected far entry (slot 90), got root=%x slot=%d ok=%v", root, slot, ok)
	}

	// Discarding the current maximum must promote the next-farthest entry.
	b.DiscardSubtree(far)
	root, slot, ok = b.HighestSlotEntry()
	if !ok || root != mid || slot != 50 {
		t.Fatalf("expected mid entry (slot 50) after discard, got root=%x slot=%d ok=%v", root, slot, ok)
	}

	b.ClearEntry(mid)
	root, slot, ok = b.HighestSlotEntry()
	if !ok || root != near || slot != 10 {
		t.Fatalf("expected near entry (slot 10) after clear, got root=%x slot=%d ok=%v", root, slot, ok)
	}
}

func TestBlockBuffer_DiscardSubtreeClearsSlots(t *testing.T) {
	b := NewBlockBuffer()

	parent := [32]byte{0x01}
	child := [32]byte{0x02}
	grandchild := [32]byte{0x03}

	b.AddChild(parent, child)
	b.SetParent(child, parent)
	b.SetSlot(child, 5)
	b.AddChild(child, grandchild)
	b.SetParent(grandchild, child)
	b.SetSlot(grandchild, 6)

	b.DiscardSubtree(child)

	if _, _, ok := b.HighestSlotEntry(); ok {
		t.Fatal("expected slot tracking cleared for the whole discarded subtree")
	}
}
