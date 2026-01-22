// Package transition implements state transition logic.
package transition

import (
	"errors"

	"github.com/devylongs/gean/types"
)

var (
	ErrSlotMismatch       = errors.New("block slot mismatch")
	ErrBlockTooOld        = errors.New("block is older than latest header")
	ErrInvalidProposer    = errors.New("incorrect block proposer")
	ErrParentRootMismatch = errors.New("block parent root mismatch")
	ErrInvalidStateRoot   = errors.New("invalid block state root")
	ErrSlotNotInFuture    = errors.New("target slot must be in the future")
)

var ZeroRoot = types.Root{}

// ProcessSlot caches the state root into the latest block header if empty.
func ProcessSlot(state *types.State) (*types.State, error) {
	if state.LatestBlockHeader.StateRoot.IsZero() {
		previousStateRoot, err := state.HashTreeRoot()
		if err != nil {
			return nil, err
		}
		newState := copyState(state)
		newState.LatestBlockHeader.StateRoot = previousStateRoot
		return newState, nil
	}
	return state, nil
}

// ProcessSlots advances the state through empty slots up to targetSlot.
func ProcessSlots(state *types.State, targetSlot types.Slot) (*types.State, error) {
	if state.Slot >= targetSlot {
		return nil, ErrSlotNotInFuture
	}

	currentState := state
	for currentState.Slot < targetSlot {
		newState, err := ProcessSlot(currentState)
		if err != nil {
			return nil, err
		}
		newState = copyState(newState)
		newState.Slot++
		currentState = newState
	}
	return currentState, nil
}

// ProcessBlockHeader validates the block header and updates state.
func ProcessBlockHeader(state *types.State, block *types.Block) (*types.State, error) {
	parentHeader := &state.LatestBlockHeader

	// Validation

	if block.Slot != state.Slot {
		return nil, ErrSlotMismatch
	}
	if block.Slot <= parentHeader.Slot {
		return nil, ErrBlockTooOld
	}
	if !types.IsProposer(types.ValidatorIndex(block.ProposerIndex), block.Slot, state.Config.NumValidators) {
		return nil, ErrInvalidProposer
	}
	parentRoot, err := parentHeader.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	if block.ParentRoot != parentRoot {
		return nil, ErrParentRootMismatch
	}

	// State Updates

	newState := copyState(state)

	// Special case: first block after genesis
	isGenesisParent := parentHeader.Slot == 0
	if isGenesisParent {
		newState.LatestJustified.Root = block.ParentRoot
		newState.LatestFinalized.Root = block.ParentRoot
	}

	numEmptySlots := int(block.Slot - parentHeader.Slot - 1)

	newState.HistoricalBlockHashes = append(newState.HistoricalBlockHashes, block.ParentRoot)
	newState.JustifiedSlots = appendBit(newState.JustifiedSlots, isGenesisParent)

	// Fill empty slots with zero hashes
	for i := 0; i < numEmptySlots; i++ {
		newState.HistoricalBlockHashes = append(newState.HistoricalBlockHashes, ZeroRoot)
		newState.JustifiedSlots = appendBit(newState.JustifiedSlots, false)
	}

	bodyRoot, err := block.Body.HashTreeRoot()
	if err != nil {
		return nil, err
	}

	newState.LatestBlockHeader = types.BlockHeader{
		Slot:          block.Slot,
		ProposerIndex: block.ProposerIndex,
		ParentRoot:    block.ParentRoot,
		StateRoot:     ZeroRoot, // Filled on next ProcessSlot
		BodyRoot:      bodyRoot,
	}

	return newState, nil
}

// ProcessAttestations applies votes and updates justification/finalization.
func ProcessAttestations(state *types.State, attestations []types.SignedVote) (*types.State, error) {
	newState := copyState(state)

	for _, signedVote := range attestations {
		vote := signedVote.Data
		source := vote.Source
		target := vote.Target

		// Source must come before target
		if source.Slot >= target.Slot {
			continue
		}

		sourceSlot := int(source.Slot)
		targetSlot := int(target.Slot)

		// Source must be within our justified history
		if sourceSlot >= bitlistLen(newState.JustifiedSlots) {
			continue
		}

		sourceIsJustified := getBit(newState.JustifiedSlots, sourceSlot)

		if sourceIsJustified && targetSlot < bitlistLen(newState.JustifiedSlots) && getBit(newState.JustifiedSlots, targetSlot) {
			// Both source and target justified - check for finalization
			// Consecutive justified checkpoints -> finalize source
			if int(source.Slot)+1 == int(target.Slot) && int(newState.LatestJustified.Slot) < targetSlot {
				newState.LatestFinalized = source
				newState.LatestJustified = target
			}
		} else if sourceIsJustified {
			// Source is justified - try to justify the target
			// Extend justified_slots if needed
			for bitlistLen(newState.JustifiedSlots) <= targetSlot {
				newState.JustifiedSlots = appendBit(newState.JustifiedSlots, false)
			}
			newState.JustifiedSlots = setBit(newState.JustifiedSlots, targetSlot, true)

			if target.Slot > newState.LatestJustified.Slot {
				newState.LatestJustified = target
			}
		}
	}

	return newState, nil
}

// ProcessBlock applies block header and attestation processing.
func ProcessBlock(state *types.State, block *types.Block) (*types.State, error) {
	newState, err := ProcessBlockHeader(state, block)
	if err != nil {
		return nil, err
	}
	return ProcessAttestations(newState, block.Body.Attestations)
}

// StateTransition applies the complete state transition for a signed block.
func StateTransition(state *types.State, signedBlock *types.SignedBlock, validateSignatures bool) (*types.State, error) {
	block := &signedBlock.Message

	// Process empty slots up to block slot
	newState, err := ProcessSlots(state, block.Slot)
	if err != nil {
		return nil, err
	}

	// Process the block
	newState, err = ProcessBlock(newState, block)
	if err != nil {
		return nil, err
	}

	// Verify state root
	computedStateRoot, err := newState.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	if block.StateRoot != computedStateRoot {
		return nil, ErrInvalidStateRoot
	}

	return newState, nil
}

// copyState creates a deep copy of the state to avoid mutation.
func copyState(state *types.State) *types.State {
	newState := *state

	if len(state.HistoricalBlockHashes) > 0 {
		newState.HistoricalBlockHashes = make([]types.Root, len(state.HistoricalBlockHashes))
		copy(newState.HistoricalBlockHashes, state.HistoricalBlockHashes)
	}
	if len(state.JustifiedSlots) > 0 {
		newState.JustifiedSlots = make([]byte, len(state.JustifiedSlots))
		copy(newState.JustifiedSlots, state.JustifiedSlots)
	}
	if len(state.JustificationsRoots) > 0 {
		newState.JustificationsRoots = make([]types.Root, len(state.JustificationsRoots))
		copy(newState.JustificationsRoots, state.JustificationsRoots)
	}
	if len(state.JustificationsValidators) > 0 {
		newState.JustificationsValidators = make([]byte, len(state.JustificationsValidators))
		copy(newState.JustificationsValidators, state.JustificationsValidators)
	}

	return &newState
}

// SSZ Bitlist Helpers
//
// SSZ bitlists use a sentinel bit to mark the length. The highest set bit
// in the last byte is the sentinel, not actual data.
//
// Example: [0b00001101] represents bits [1, 0, 1] with sentinel at position 3
//          [0b00000001] represents an empty bitlist (just sentinel)

// bitlistLen returns the number of data bits (excluding sentinel).
func bitlistLen(bitlist []byte) int {
	if len(bitlist) == 0 {
		return 0
	}
	lastByte := bitlist[len(bitlist)-1]
	if lastByte == 0 {
		return 0
	}
	// Find sentinel position (highest set bit in last byte)
	sentinelPos := 7
	for ; sentinelPos >= 0 && (lastByte&(1<<sentinelPos)) == 0; sentinelPos-- {
	}
	return (len(bitlist)-1)*8 + sentinelPos
}

// getBit returns the value of bit at index.
func getBit(bitlist []byte, index int) bool {
	if index < 0 || index >= bitlistLen(bitlist) {
		return false
	}
	byteIndex := index / 8
	bitIndex := index % 8
	return (bitlist[byteIndex] & (1 << bitIndex)) != 0
}

// setBit sets the bit at index and returns a new bitlist.
func setBit(bitlist []byte, index int, value bool) []byte {
	if index < 0 || index >= bitlistLen(bitlist) {
		return bitlist
	}
	byteIndex := index / 8
	bitIndex := index % 8

	newBitlist := make([]byte, len(bitlist))
	copy(newBitlist, bitlist)

	if value {
		newBitlist[byteIndex] |= (1 << bitIndex)
	} else {
		newBitlist[byteIndex] &^= (1 << bitIndex)
	}
	return newBitlist
}

// appendBit appends a bit to the bitlist and returns a new bitlist.
//
// Steps:
// 1. Calculate new length and required bytes
// 2. Copy existing data bits (without old sentinel)
// 3. Set the new bit value
// 4. Set the new sentinel bit
func appendBit(bitlist []byte, value bool) []byte {
	length := bitlistLen(bitlist)
	newLength := length + 1
	newByteLen := (newLength + 8) / 8 // Ceiling division for bytes needed

	var newBitlist []byte

	if newByteLen > len(bitlist) {
		// Need more bytes - allocate and copy data bits
		newBitlist = make([]byte, newByteLen)
		if len(bitlist) > 0 {
			// Copy all complete bytes except last
			copy(newBitlist, bitlist[:len(bitlist)-1])
			// Copy data bits from last byte (mask out sentinel)
			if length > 0 {
				lastByteIndex := (length - 1) / 8
				if lastByteIndex >= len(bitlist)-1 {
					sentinelPos := length % 8
					if sentinelPos == 0 {
						sentinelPos = 8
					}
					mask := byte((1 << sentinelPos) - 1)
					newBitlist[lastByteIndex] = bitlist[lastByteIndex] & mask
				}
			}
		}
	} else {
		// Same number of bytes - copy and clear old sentinel
		newBitlist = make([]byte, len(bitlist))
		copy(newBitlist, bitlist)
		sentinelByteIndex := length / 8
		sentinelBitIndex := length % 8
		if sentinelByteIndex < len(newBitlist) {
			newBitlist[sentinelByteIndex] &^= (1 << sentinelBitIndex)
		}
	}

	// Set the new data bit
	bitByteIndex := length / 8
	bitBitIndex := length % 8
	if value {
		newBitlist[bitByteIndex] |= (1 << bitBitIndex)
	}

	// Set new sentinel bit
	sentinelByteIndex := newLength / 8
	sentinelBitIndex := newLength % 8
	newBitlist[sentinelByteIndex] |= (1 << sentinelBitIndex)

	return newBitlist
}
