package blockprocessor

import (
	"fmt"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func validatorAt(state *types.State, index uint64) (*types.Validator, error) {
	if state == nil {
		return nil, &store.StoreError{Kind: store.ErrMissingParentState, Message: "state missing"}
	}
	if index >= uint64(len(state.Validators)) {
		return nil, &store.StoreError{
			Kind:    store.ErrInvalidValidatorIndex,
			Message: fmt.Sprintf("validator %d out of range", index),
		}
	}
	validator := state.Validators[index]
	if validator == nil {
		return nil, &store.StoreError{
			Kind:    store.ErrInvalidValidatorIndex,
			Message: fmt.Sprintf("validator %d is nil", index),
		}
	}
	return validator, nil
}
