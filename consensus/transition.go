package consensus

import "fmt"

var ZeroHash = Root{}

// ProcessSlot performs per-slot maintenance.
// If the latest block header has an empty state_root, fill it with the current state root.
func (s *State) ProcessSlot() (*State, error) {
	if s.LatestBlockHeader.StateRoot.IsZero() {
		stateRoot, err := s.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("hash state: %w", err)
		}
		newState := s.Copy()
		newState.LatestBlockHeader.StateRoot = stateRoot
		return newState, nil
	}
	return s, nil
}

// ProcessSlots advances the state through empty slots up to targetSlot.
func (s *State) ProcessSlots(targetSlot Slot) (*State, error) {
	if s.Slot >= targetSlot {
		return nil, fmt.Errorf("target slot %d must be greater than current slot %d", targetSlot, s.Slot)
	}

	state := s
	var err error
	for state.Slot < targetSlot {
		state, err = state.ProcessSlot()
		if err != nil {
			return nil, err
		}
		newState := state.Copy()
		newState.Slot++
		state = newState
	}
	return state, nil
}

// ProcessBlockHeader validates and applies a block header.
func (s *State) ProcessBlockHeader(block *Block) (*State, error) {
	// Validate slot matches
	if block.Slot != s.Slot {
		return nil, fmt.Errorf("block slot %d != state slot %d", block.Slot, s.Slot)
	}

	// Block must be newer than latest header
	if block.Slot <= s.LatestBlockHeader.Slot {
		return nil, fmt.Errorf("block slot %d <= latest header slot %d", block.Slot, s.LatestBlockHeader.Slot)
	}

	// Validate proposer
	if !s.IsProposer(ValidatorIndex(block.ProposerIndex)) {
		return nil, fmt.Errorf("invalid proposer %d for slot %d", block.ProposerIndex, block.Slot)
	}

	// Validate parent root
	expectedParent, err := s.LatestBlockHeader.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash latest header: %w", err)
	}
	if block.ParentRoot != expectedParent {
		return nil, fmt.Errorf("parent root mismatch")
	}

	newState := s.Copy()

	// First block after genesis: mark genesis as justified and finalized
	if s.LatestBlockHeader.Slot == 0 {
		newState.LatestJustified.Root = block.ParentRoot
		newState.LatestFinalized.Root = block.ParentRoot
	}

	// Append parent root to history
	newState.HistoricalBlockHashes = append(newState.HistoricalBlockHashes, block.ParentRoot)

	// Track justified slot (genesis slot 0 is always justified)
	parentSlot := int(s.LatestBlockHeader.Slot)
	newState.JustifiedSlots = appendBitAt(newState.JustifiedSlots, parentSlot, s.LatestBlockHeader.Slot == 0)

	// Fill empty slots with zero hashes
	emptySlots := int(block.Slot - s.LatestBlockHeader.Slot - 1)
	for i := 0; i < emptySlots; i++ {
		newState.HistoricalBlockHashes = append(newState.HistoricalBlockHashes, ZeroHash)
		emptySlot := parentSlot + 1 + i
		newState.JustifiedSlots = appendBitAt(newState.JustifiedSlots, emptySlot, false)
	}

	// Create new block header (state_root left empty, filled by next ProcessSlot)
	bodyRoot, err := block.Body.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash body: %w", err)
	}
	newState.LatestBlockHeader = BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     Root{},
		BodyRoot:      bodyRoot,
	}

	return newState, nil
}

// ProcessAttestations processes attestation votes.
func (s *State) ProcessAttestations(attestations []SignedVote) (*State, error) {
	newState := s.Copy()

	for _, signed := range attestations {
		vote := signed.Data

		// Source must be before target
		if vote.Source.Slot >= vote.Target.Slot {
			continue
		}

		sourceSlot := int(vote.Source.Slot)
		targetSlot := int(vote.Target.Slot)

		// Check source is justified
		if sourceSlot >= len(newState.JustifiedSlots)*8 {
			continue
		}
		if !getBit(newState.JustifiedSlots, sourceSlot) {
			continue
		}

		// Ensure justified slots is long enough
		for len(newState.JustifiedSlots)*8 <= targetSlot {
			newState.JustifiedSlots = append(newState.JustifiedSlots, 0)
		}

		// Mark target as justified
		newState.JustifiedSlots = setBit(newState.JustifiedSlots, targetSlot, true)

		// Update latest justified if newer
		if vote.Target.Slot > newState.LatestJustified.Slot {
			newState.LatestJustified = vote.Target
		}

		// Check for finalization (consecutive justified)
		if vote.Source.Slot+1 == vote.Target.Slot &&
			newState.LatestJustified.Slot < vote.Target.Slot {
			newState.LatestFinalized = vote.Source
			newState.LatestJustified = vote.Target
		}
	}

	return newState, nil
}

// ProcessBlock applies full block processing.
func (s *State) ProcessBlock(block *Block) (*State, error) {
	state, err := s.ProcessBlockHeader(block)
	if err != nil {
		return nil, err
	}
	return state.ProcessAttestations(block.Body.Attestations)
}

// StateTransition applies the complete state transition for a signed block.
func (s *State) StateTransition(signedBlock *SignedBlock, validateSignatures bool) (*State, error) {
	if validateSignatures {
		// TODO: signature validation
	}

	block := &signedBlock.Message

	// Process slots up to block slot
	state, err := s.ProcessSlots(block.Slot)
	if err != nil {
		return nil, err
	}

	// Process the block
	newState, err := state.ProcessBlock(block)
	if err != nil {
		return nil, err
	}

	// Validate state root
	computedRoot, err := newState.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash new state: %w", err)
	}
	if block.StateRoot != computedRoot {
		return nil, fmt.Errorf("state root mismatch: expected %x, got %x", block.StateRoot, computedRoot)
	}

	return newState, nil
}

// Copy creates a deep copy of the state.
func (s *State) Copy() *State {
	cp := *s
	cp.HistoricalBlockHashes = append([]Root{}, s.HistoricalBlockHashes...)
	cp.JustifiedSlots = append([]byte{}, s.JustifiedSlots...)
	cp.JustificationRoots = append([]Root{}, s.JustificationRoots...)
	cp.JustificationValidators = append([]byte{}, s.JustificationValidators...)
	return &cp
}

// Bitlist helpers

// appendBitAt sets a bit at the given index, extending the slice if needed.
func appendBitAt(bits []byte, index int, val bool) []byte {
	byteIndex := index / 8
	for byteIndex >= len(bits) {
		bits = append(bits, 0)
	}
	if val {
		bits[byteIndex] |= 1 << (index % 8)
	}
	return bits
}

func getBit(bits []byte, index int) bool {
	if index/8 >= len(bits) {
		return false
	}
	return bits[index/8]&(1<<(index%8)) != 0
}

func setBit(bits []byte, index int, val bool) []byte {
	for index/8 >= len(bits) {
		bits = append(bits, 0)
	}
	if val {
		bits[index/8] |= 1 << (index % 8)
	} else {
		bits[index/8] &^= 1 << (index % 8)
	}
	return bits
}
