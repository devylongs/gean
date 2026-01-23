package consensus

// GetForkChoiceHead uses LMD GHOST to find the head block from a given root.
// It walks down the tree, at each fork choosing the child with the most votes.
func GetForkChoiceHead(blocks map[Root]*Block, root Root, latestVotes map[ValidatorIndex]Checkpoint, minScore int) Root {
	// Start at genesis if root is zero
	if root.IsZero() {
		var minSlot Slot = ^Slot(0)
		for hash, block := range blocks {
			if block.Slot < minSlot {
				minSlot = block.Slot
				root = hash
			}
		}
	}

	// No votes means return starting root
	if len(latestVotes) == 0 {
		return root
	}

	// Count votes for each block (votes for descendants count for ancestors)
	voteWeights := make(map[Root]int)
	rootSlot := blocks[root].Slot

	for _, vote := range latestVotes {
		if _, exists := blocks[vote.Root]; !exists {
			continue
		}

		// Walk up from vote target, incrementing ancestor weights
		blockHash := vote.Root
		for blocks[blockHash].Slot > rootSlot {
			voteWeights[blockHash]++
			blockHash = blocks[blockHash].ParentRoot
		}
	}

	// Build children mapping for blocks above min score
	childrenMap := make(map[Root][]Root)
	for blockHash, block := range blocks {
		if !block.ParentRoot.IsZero() && voteWeights[blockHash] >= minScore {
			childrenMap[block.ParentRoot] = append(childrenMap[block.ParentRoot], blockHash)
		}
	}

	// Walk down tree, choosing child with most votes
	current := root
	for {
		children := childrenMap[current]
		if len(children) == 0 {
			return current
		}

		// Choose best child: most votes, then highest slot, then highest hash
		best := children[0]
		bestWeight := voteWeights[best]
		bestSlot := blocks[best].Slot

		for _, child := range children[1:] {
			weight := voteWeights[child]
			slot := blocks[child].Slot

			if weight > bestWeight ||
				(weight == bestWeight && slot > bestSlot) ||
				(weight == bestWeight && slot == bestSlot && compareRoots(child, best) > 0) {
				best = child
				bestWeight = weight
				bestSlot = slot
			}
		}

		current = best
	}
}

// GetLatestJustified finds the justified checkpoint with the highest slot.
func GetLatestJustified(states map[Root]*State) *Checkpoint {
	if len(states) == 0 {
		return nil
	}

	var latest *Checkpoint
	var latestSlot Slot

	for _, state := range states {
		if latest == nil || state.LatestJustified.Slot > latestSlot {
			cp := state.LatestJustified
			latest = &cp
			latestSlot = cp.Slot
		}
	}

	return latest
}

// compareRoots compares two roots lexicographically.
func compareRoots(a, b Root) int {
	for i := 0; i < 32; i++ {
		if a[i] > b[i] {
			return 1
		}
		if a[i] < b[i] {
			return -1
		}
	}
	return 0
}
