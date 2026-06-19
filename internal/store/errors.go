package store

type StoreError struct {
	Kind    StoreErrorKind
	Message string
}

func (e *StoreError) Error() string { return e.Message }

type StoreErrorKind int

const (
	ErrMissingParentState StoreErrorKind = iota
	ErrInvalidValidatorIndex
	ErrPubkeyDecodingFailed
	ErrSignatureDecodingFailed
	ErrSignatureVerificationFailed
	ErrProposerSignatureDecodingFailed
	ErrProposerSignatureVerificationFailed
	ErrStateTransitionFailed
	ErrUnknownSourceBlock
	ErrUnknownTargetBlock
	ErrUnknownHeadBlock
	ErrSourceExceedsTarget
	ErrHeadOlderThanTarget
	ErrSourceSlotMismatch
	ErrTargetSlotMismatch
	ErrHeadSlotMismatch
	ErrAttestationTooFarInFuture
	ErrAttestationSignatureMismatch
	ErrParticipantsMismatch
	ErrAggregateVerificationFailed
	ErrSignatureAggregationFailed
	ErrMissingTargetState
	ErrNotProposer
	ErrDuplicateAttestationData
	ErrJustifiedDivergenceNotClosed
	ErrSourceNotAncestorOfTarget
	ErrTargetNotAncestorOfHead
	ErrAttestationSlotBeforeHead
)
