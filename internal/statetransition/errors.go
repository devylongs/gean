package statetransition

import "fmt"

type StateSlotIsNewerError struct {
	TargetSlot  uint64
	CurrentSlot uint64
}

func (e *StateSlotIsNewerError) Error() string {
	return fmt.Sprintf("state slot %d >= target slot %d", e.CurrentSlot, e.TargetSlot)
}

type SlotMismatchError struct {
	StateSlot uint64
	BlockSlot uint64
}

func (e *SlotMismatchError) Error() string {
	return fmt.Sprintf("state slot %d != block slot %d", e.StateSlot, e.BlockSlot)
}

type ParentSlotIsNewerError struct {
	ParentSlot uint64
	BlockSlot  uint64
}

func (e *ParentSlotIsNewerError) Error() string {
	return fmt.Sprintf("parent slot %d >= block slot %d", e.ParentSlot, e.BlockSlot)
}

type InvalidProposerError struct {
	Expected uint64
	Found    uint64
}

func (e *InvalidProposerError) Error() string {
	return fmt.Sprintf("invalid proposer: expected %d, got %d", e.Expected, e.Found)
}

type InvalidParentError struct {
	Expected [32]byte
	Found    [32]byte
}

func (e *InvalidParentError) Error() string {
	return fmt.Sprintf("invalid parent root: expected %x, got %x", e.Expected[:4], e.Found[:4])
}

type StateRootMismatchError struct {
	Expected [32]byte
	Computed [32]byte
}

func (e *StateRootMismatchError) Error() string {
	return fmt.Sprintf("state root mismatch: block has %x, computed %x", e.Expected[:4], e.Computed[:4])
}

type SlotGapTooLargeError struct {
	Gap     uint64
	Current uint64
	Max     uint64
}

func (e *SlotGapTooLargeError) Error() string {
	return fmt.Sprintf("slot gap %d at slot %d exceeds max %d", e.Gap, e.Current, e.Max)
}

type AttesterIndexOutOfRangeError struct {
	Index      uint64
	Validators uint64
}

func (e *AttesterIndexOutOfRangeError) Error() string {
	return fmt.Sprintf("attestation aggregation bit %d outside validator registry of %d", e.Index, e.Validators)
}

// TooManyAttestationDataError rejects a block whose distinct AttestationData count
// exceeds the per-block cap. The bound is a property of the transition itself so it
// holds for raw state transitions and block-production trial blocks alike, not only
// the import caller. Split aggregates that share one data entry count once.
type TooManyAttestationDataError struct {
	Count uint64
	Max   uint64
}

func (e *TooManyAttestationDataError) Error() string {
	return fmt.Sprintf("block contains %d distinct AttestationData entries; maximum is %d", e.Count, e.Max)
}

// JustifiedSlotOutOfRangeError rejects a block whose attestation queries the
// justification status of an active slot that lies beyond the tracked bitfield.
// Slots at or below the finalized boundary are justified by definition and never
// reach this path; only an in-future-but-untracked slot is a domain rejection
// rather than a silently dropped vote.
type JustifiedSlotOutOfRangeError struct {
	Slot              uint64
	FinalizedBoundary uint64
	TrackedLength     uint64
}

func (e *JustifiedSlotOutOfRangeError) Error() string {
	return fmt.Sprintf("Slot %d is outside the tracked range (finalized_boundary=%d, tracked_length=%d)",
		e.Slot, e.FinalizedBoundary, e.TrackedLength)
}

var ErrEmptyAggregationBits = fmt.Errorf("attestation aggregation bits have no set bits")

var ErrNoValidators = fmt.Errorf("state has no validators")
var ErrZeroHashInJustificationRoots = fmt.Errorf("zero hash found in justifications_roots")
var ErrMalformedState = fmt.Errorf("malformed state")
var ErrMalformedBlock = fmt.Errorf("malformed block")

func malformedState(field string) error {
	return fmt.Errorf("%w: %s is nil", ErrMalformedState, field)
}

func malformedBlock(field string) error {
	return fmt.Errorf("%w: %s is nil", ErrMalformedBlock, field)
}
