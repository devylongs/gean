package pending

type BlockBuffer struct {
	children map[[32]byte]map[[32]byte]bool
	parents  map[[32]byte][32]byte
	depths   map[[32]byte]int
	slots    map[[32]byte]uint64
}

func NewBlockBuffer() *BlockBuffer {
	b := &BlockBuffer{}
	b.ensureMaps()
	return b
}

func (b *BlockBuffer) Count() int {
	if b == nil {
		return 0
	}
	n := 0
	for _, set := range b.children {
		n += len(set)
	}
	return n
}

func (b *BlockBuffer) ParentBuckets() int {
	if b == nil {
		return 0
	}
	return len(b.children)
}

func (b *BlockBuffer) Entries() int {
	if b == nil {
		return 0
	}
	return len(b.parents)
}

func (b *BlockBuffer) ChildCount(parent [32]byte) int {
	if b == nil {
		return 0
	}
	return len(b.children[parent])
}

func (b *BlockBuffer) Depth(root [32]byte) (int, bool) {
	if b == nil {
		return 0, false
	}
	d, ok := b.depths[root]
	return d, ok
}

func (b *BlockBuffer) SetDepth(root [32]byte, depth int) {
	if !b.ensureMaps() {
		return
	}
	b.depths[root] = depth
}

func (b *BlockBuffer) ClearDepth(root [32]byte) {
	if b == nil {
		return
	}
	delete(b.depths, root)
}

func (b *BlockBuffer) SetSlot(root [32]byte, slot uint64) {
	if !b.ensureMaps() {
		return
	}
	b.slots[root] = slot
}

// HighestSlotEntry returns the tracked entry farthest from the known chain —
// the natural eviction victim when the buffer is full, since entries closer
// to a known state are the ones that can actually connect.
func (b *BlockBuffer) HighestSlotEntry() ([32]byte, uint64, bool) {
	if b == nil || len(b.slots) == 0 {
		return [32]byte{}, 0, false
	}
	var maxRoot [32]byte
	var maxSlot uint64
	found := false
	for root, slot := range b.slots {
		if !found || slot > maxSlot {
			maxRoot, maxSlot, found = root, slot, true
		}
	}
	return maxRoot, maxSlot, found
}

func (b *BlockBuffer) ResolveAncestor(start [32]byte) [32]byte {
	if b == nil {
		return start
	}
	root := start
	seen := make(map[[32]byte]bool)
	for {
		if seen[root] {
			return root
		}
		seen[root] = true
		anc, ok := b.parents[root]
		if !ok {
			return root
		}
		root = anc
	}
}

func (b *BlockBuffer) SetParent(root, ancestor [32]byte) {
	if !b.ensureMaps() {
		return
	}
	b.parents[root] = ancestor
}

func (b *BlockBuffer) AddChild(parent, child [32]byte) {
	if !b.ensureMaps() {
		return
	}
	set, ok := b.children[parent]
	if !ok {
		set = make(map[[32]byte]bool)
		b.children[parent] = set
	}
	set[child] = true
}

func (b *BlockBuffer) RemoveBucket(parent [32]byte) (map[[32]byte]bool, bool) {
	if b == nil {
		return nil, false
	}
	set, ok := b.children[parent]
	if !ok {
		return nil, false
	}
	delete(b.children, parent)
	return set, true
}

func (b *BlockBuffer) RemoveChild(parent, child [32]byte) {
	if b == nil {
		return
	}
	set, ok := b.children[parent]
	if !ok {
		return
	}
	delete(set, child)
	if len(set) == 0 {
		delete(b.children, parent)
	}
}

func (b *BlockBuffer) ClearEntry(root [32]byte) {
	if b == nil {
		return
	}
	delete(b.parents, root)
	delete(b.depths, root)
	delete(b.slots, root)
}

func (b *BlockBuffer) Pairs() [][2][32]byte {
	if b == nil {
		return nil
	}
	out := make([][2][32]byte, 0, b.Count())
	for parent, set := range b.children {
		for child := range set {
			out = append(out, [2][32]byte{parent, child})
		}
	}
	return out
}

func (b *BlockBuffer) DiscardSubtree(root [32]byte) {
	if b == nil {
		return
	}
	if parent, ok := b.parents[root]; ok {
		b.RemoveChild(parent, root)
	} else {
		for parent, set := range b.children {
			if set[root] {
				b.RemoveChild(parent, root)
				break
			}
		}
	}
	b.discardSubtree(root)
}

func (b *BlockBuffer) discardSubtree(root [32]byte) {
	if b == nil {
		return
	}
	delete(b.parents, root)
	delete(b.depths, root)
	delete(b.slots, root)
	set, ok := b.children[root]
	if !ok {
		return
	}
	delete(b.children, root)
	for child := range set {
		b.discardSubtree(child)
	}
}

func (b *BlockBuffer) ensureMaps() bool {
	if b == nil {
		return false
	}
	if b.children == nil {
		b.children = make(map[[32]byte]map[[32]byte]bool)
	}
	if b.parents == nil {
		b.parents = make(map[[32]byte][32]byte)
	}
	if b.depths == nil {
		b.depths = make(map[[32]byte]int)
	}
	if b.slots == nil {
		b.slots = make(map[[32]byte]uint64)
	}
	return true
}
