package consensus

import "fmt"

// Store tracks all information required for the LMD GHOST fork choice algorithm.
type Store struct {
	Time            uint64
	Config          Config
	Head            Root
	SafeTarget      Root
	LatestJustified Checkpoint
	LatestFinalized Checkpoint

	Blocks           map[Root]*Block
	States           map[Root]*State
	LatestKnownVotes map[ValidatorIndex]Checkpoint
	LatestNewVotes   map[ValidatorIndex]Checkpoint
}

// NewStore initializes a fork choice store from an anchor state and block.
func NewStore(state *State, anchorBlock *Block) (*Store, error) {
	stateRoot, err := state.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash state: %w", err)
	}

	if anchorBlock.StateRoot != stateRoot {
		return nil, fmt.Errorf("anchor block state root mismatch")
	}

	anchorRoot, err := anchorBlock.HashTreeRoot()
	if err != nil {
		return nil, fmt.Errorf("hash anchor block: %w", err)
	}

	return &Store{
		Time:             uint64(anchorBlock.Slot) * IntervalsPerSlot,
		Config:           state.Config,
		Head:             anchorRoot,
		SafeTarget:       anchorRoot,
		LatestJustified:  state.LatestJustified,
		LatestFinalized:  state.LatestFinalized,
		Blocks:           map[Root]*Block{anchorRoot: anchorBlock},
		States:           map[Root]*State{anchorRoot: state},
		LatestKnownVotes: make(map[ValidatorIndex]Checkpoint),
		LatestNewVotes:   make(map[ValidatorIndex]Checkpoint),
	}, nil
}

// ProcessBlock adds a new block and updates fork choice state.
func (s *Store) ProcessBlock(block *Block) error {
	blockHash, err := block.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("hash block: %w", err)
	}

	// Skip if already known
	if _, exists := s.Blocks[blockHash]; exists {
		return nil
	}

	// Get parent state
	parentState, exists := s.States[block.ParentRoot]
	if !exists {
		return fmt.Errorf("parent state not found")
	}

	// Apply state transition
	newState, err := parentState.ProcessSlots(block.Slot)
	if err != nil {
		return fmt.Errorf("process slots: %w", err)
	}
	newState, err = newState.ProcessBlock(block)
	if err != nil {
		return fmt.Errorf("process block: %w", err)
	}

	// Store block and state
	s.Blocks[blockHash] = block
	s.States[blockHash] = newState

	// Process attestations
	for _, signedVote := range block.Body.Attestations {
		s.processAttestation(&signedVote, true)
	}

	// Update head
	s.UpdateHead()
	return nil
}

// processAttestation handles a new attestation vote.
func (s *Store) processAttestation(signedVote *SignedVote, isFromBlock bool) {
	vote := signedVote.Data
	validatorID := ValidatorIndex(vote.ValidatorID)

	if isFromBlock {
		// On-chain attestation
		if known, exists := s.LatestKnownVotes[validatorID]; !exists || known.Slot < vote.Slot {
			s.LatestKnownVotes[validatorID] = vote.Target
		}
		if newVote, exists := s.LatestNewVotes[validatorID]; exists && newVote.Slot <= vote.Target.Slot {
			delete(s.LatestNewVotes, validatorID)
		}
	} else {
		// Network gossip attestation
		if newVote, exists := s.LatestNewVotes[validatorID]; !exists || newVote.Slot < vote.Target.Slot {
			s.LatestNewVotes[validatorID] = vote.Target
		}
	}
}

// UpdateHead updates the store's head based on latest justified checkpoint and votes.
func (s *Store) UpdateHead() {
	if latest := GetLatestJustified(s.States); latest != nil {
		s.LatestJustified = *latest
	}

	s.Head = GetForkChoiceHead(s.Blocks, s.LatestJustified.Root, s.LatestKnownVotes, 0)

	if state, exists := s.States[s.Head]; exists {
		s.LatestFinalized = state.LatestFinalized
	}
}

// AcceptNewVotes moves pending votes to known votes and updates head.
func (s *Store) AcceptNewVotes() {
	for validatorID, vote := range s.LatestNewVotes {
		s.LatestKnownVotes[validatorID] = vote
	}
	s.LatestNewVotes = make(map[ValidatorIndex]Checkpoint)
	s.UpdateHead()
}

// UpdateSafeTarget calculates the safe target with 2/3 majority threshold.
func (s *Store) UpdateSafeTarget() {
	minScore := int((s.Config.NumValidators*2 + 2) / 3) // ceiling division
	s.SafeTarget = GetForkChoiceHead(s.Blocks, s.LatestJustified.Root, s.LatestNewVotes, minScore)
}

// TickInterval advances store time by one interval.
func (s *Store) TickInterval(hasProposal bool) {
	s.Time++
	currentInterval := s.Time % IntervalsPerSlot

	switch currentInterval {
	case 0:
		if hasProposal {
			s.AcceptNewVotes()
		}
	case 1:
		// Validator voting interval - no action
	case 2:
		s.UpdateSafeTarget()
	default:
		s.AcceptNewVotes()
	}
}

// AdvanceTime ticks the store forward to the given time.
func (s *Store) AdvanceTime(time uint64, hasProposal bool) {
	tickIntervalTime := (time - s.Config.GenesisTime) / SecondsPerInterval

	for s.Time < tickIntervalTime {
		shouldSignal := hasProposal && (s.Time+1) == tickIntervalTime
		s.TickInterval(shouldSignal)
	}
}

// GetProposalHead returns the head for block proposal at the given slot.
func (s *Store) GetProposalHead(slot Slot) Root {
	slotTime := s.Config.GenesisTime + uint64(slot)*SecondsPerSlot
	s.AdvanceTime(slotTime, true)
	s.AcceptNewVotes()
	return s.Head
}

// GetVoteTarget calculates the target checkpoint for validator votes.
func (s *Store) GetVoteTarget() Checkpoint {
	targetRoot := s.Head

	// Walk back up to 3 steps if safe target is newer
	for i := 0; i < 3; i++ {
		if s.Blocks[targetRoot].Slot > s.Blocks[s.SafeTarget].Slot {
			targetRoot = s.Blocks[targetRoot].ParentRoot
		}
	}

	// Ensure target is in justifiable slot range
	for !s.Blocks[targetRoot].Slot.IsJustifiableAfter(s.LatestFinalized.Slot) {
		targetRoot = s.Blocks[targetRoot].ParentRoot
	}

	block := s.Blocks[targetRoot]
	blockRoot, _ := block.HashTreeRoot()
	return Checkpoint{Root: blockRoot, Slot: block.Slot}
}

// CurrentSlot returns the current slot based on store time.
func (s *Store) CurrentSlot() Slot {
	return Slot(s.Time / IntervalsPerSlot)
}
