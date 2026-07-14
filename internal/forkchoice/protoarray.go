package forkchoice

import (
	"bytes"
	"sort"
)

type ProtoNode struct {
	Slot           uint64
	Root           [32]byte
	ParentRoot     [32]byte
	Parent         int
	Weight         int64
	BestChild      int
	BestDescendant int
}

type ProtoArray struct {
	nodes   []ProtoNode
	indices map[[32]byte]int
}

func NewProtoArray(anchorSlot uint64, anchorRoot, anchorParentRoot [32]byte) *ProtoArray {
	pa := &ProtoArray{
		indices: make(map[[32]byte]int),
	}
	pa.nodes = append(pa.nodes, ProtoNode{
		Slot:           anchorSlot,
		Root:           anchorRoot,
		ParentRoot:     anchorParentRoot,
		Parent:         -1,
		Weight:         0,
		BestChild:      -1,
		BestDescendant: -1,
	})
	pa.indices[anchorRoot] = 0
	return pa
}

func (pa *ProtoArray) Nodes() []ProtoNode {
	if pa == nil {
		return nil
	}
	out := make([]ProtoNode, len(pa.nodes))
	copy(out, pa.nodes)
	return out
}

func (pa *ProtoArray) OnBlock(slot uint64, root, parentRoot [32]byte) {
	if pa == nil || root == parentRoot {
		return
	}
	if pa.indices == nil {
		pa.indices = make(map[[32]byte]int)
	}
	if _, exists := pa.indices[root]; exists {
		return
	}
	nodeIndex := len(pa.nodes)
	parentIdx := -1
	if idx, ok := pa.indices[parentRoot]; ok {
		if slot <= pa.nodes[idx].Slot {
			return
		}
		parentIdx = idx
	}

	pa.nodes = append(pa.nodes, ProtoNode{
		Slot:           slot,
		Root:           root,
		ParentRoot:     parentRoot,
		Parent:         parentIdx,
		Weight:         0,
		BestChild:      -1,
		BestDescendant: -1,
	})
	pa.indices[root] = nodeIndex
	pa.linkOrphanChildren(root, nodeIndex)
}

func (pa *ProtoArray) linkOrphanChildren(parentRoot [32]byte, parentIdx int) {
	if parentIdx < 0 || parentIdx >= len(pa.nodes) {
		return
	}
	parentSlot := pa.nodes[parentIdx].Slot
	for i := range pa.nodes {
		node := &pa.nodes[i]
		if i == parentIdx || node.Parent >= 0 || node.ParentRoot != parentRoot {
			continue
		}
		if node.Slot <= parentSlot {
			continue
		}
		node.Parent = parentIdx
	}
}

func (pa *ProtoArray) ApplyScoreChanges(deltas []int64, cutoffWeight int64) {
	if pa == nil {
		return
	}
	if len(deltas) != len(pa.nodes) {
		return
	}

	order := pa.descendingSlotOrder()
	for _, i := range order {
		pa.nodes[i].Weight += deltas[i]
		if pa.nodes[i].Parent >= 0 {
			deltas[pa.nodes[i].Parent] += deltas[i]
		}
	}

	for i := range pa.nodes {
		pa.nodes[i].BestChild = -1
		pa.nodes[i].BestDescendant = -1
	}

	for _, i := range order {
		parentIdx := pa.nodes[i].Parent
		if parentIdx < 0 {
			continue
		}

		nodeBestDesc := pa.nodes[i].BestDescendant
		if nodeBestDesc < 0 {
			if pa.nodes[i].Weight >= cutoffWeight {
				nodeBestDesc = i
			} else {
				nodeBestDesc = -1
			}
		}

		parent := &pa.nodes[parentIdx]
		shouldUpdate := false

		if parent.BestChild == i {
			if parent.BestDescendant != nodeBestDesc {
				shouldUpdate = true
			}
		} else if parent.BestChild >= 0 {
			bestChild := &pa.nodes[parent.BestChild]
			if bestChild.Weight < pa.nodes[i].Weight {
				shouldUpdate = true
			} else if bestChild.Weight == pa.nodes[i].Weight {
				if bytes.Compare(bestChild.Root[:], pa.nodes[i].Root[:]) < 0 {
					shouldUpdate = true
				}
			}
		} else {
			shouldUpdate = true
		}

		if shouldUpdate {
			parent.BestChild = i
			parent.BestDescendant = nodeBestDesc
		}
	}
}

func (pa *ProtoArray) descendingSlotOrder() []int {
	order := make([]int, len(pa.nodes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		left, right := pa.nodes[order[i]], pa.nodes[order[j]]
		if left.Slot != right.Slot {
			return left.Slot > right.Slot
		}
		return order[i] > order[j]
	})
	return order
}

func (pa *ProtoArray) ascendingSlotOrder() []int {
	order := make([]int, len(pa.nodes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		left, right := pa.nodes[order[i]], pa.nodes[order[j]]
		if left.Slot != right.Slot {
			return left.Slot < right.Slot
		}
		return order[i] < order[j]
	})
	return order
}

func (pa *ProtoArray) FindHead(justifiedRoot [32]byte) [32]byte {
	if pa == nil {
		return justifiedRoot
	}
	idx, ok := pa.indices[justifiedRoot]
	if !ok {
		return justifiedRoot
	}
	bestDesc := pa.nodes[idx].BestDescendant
	if bestDesc < 0 {
		return justifiedRoot
	}
	return pa.nodes[bestDesc].Root
}

func (pa *ProtoArray) Prune(finalizedRoot [32]byte) map[int]int {
	if pa == nil {
		return nil
	}
	finalizedIdx, ok := pa.indices[finalizedRoot]
	if !ok || finalizedIdx == 0 {
		return nil
	}

	// Ascending-slot order is a topological order of the parent forest: every parent
	// edge is strictly slot-increasing (OnBlock and linkOrphanChildren reject a child
	// whose slot is not above its parent's), so a node's parent is always visited first.
	// One pass then keeps exactly the finalized subtree — pre-finalized ancestors, sibling
	// forks, and unlinked orphans fall out. Iterating in this order also leaves kept sorted,
	// so no separate sort is needed. This replaces a fixed-point iteration that degraded to
	// O(N^2) when out-of-order arrivals placed a child ahead of its parent in the array.
	order := pa.ascendingSlotOrder()
	keep := make([]bool, len(pa.nodes))
	kept := make([]int, 0, len(pa.nodes)-finalizedIdx)
	for _, i := range order {
		parent := pa.nodes[i].Parent
		if i == finalizedIdx || (parent >= 0 && parent < len(pa.nodes) && keep[parent]) {
			keep[i] = true
			kept = append(kept, i)
		}
	}

	indexMap := make(map[int]int, len(kept))
	newNodes := make([]ProtoNode, 0, len(kept))
	for _, oldIdx := range kept {
		indexMap[oldIdx] = len(newNodes)
		newNodes = append(newNodes, pa.nodes[oldIdx])
	}

	newIndices := make(map[[32]byte]int, len(newNodes))
	for i := range newNodes {
		newIndices[newNodes[i].Root] = i
		newNodes[i].Parent = remapProtoIndex(newNodes[i].Parent, indexMap)
		newNodes[i].BestChild = remapProtoIndex(newNodes[i].BestChild, indexMap)
		newNodes[i].BestDescendant = remapProtoIndex(newNodes[i].BestDescendant, indexMap)
	}
	pa.nodes = newNodes
	pa.indices = newIndices
	return indexMap
}

func remapProtoIndex(oldIdx int, indexMap map[int]int) int {
	if oldIdx < 0 {
		return -1
	}
	if newIdx, ok := indexMap[oldIdx]; ok {
		return newIdx
	}
	return -1
}

func (pa *ProtoArray) Len() int {
	if pa == nil {
		return 0
	}
	return len(pa.nodes)
}
