package blockprocessor

import (
	"fmt"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func validateStore(s *store.ConsensusStore) error {
	if s == nil {
		return fmt.Errorf("consensus store is nil")
	}
	if s.Backend == nil {
		return fmt.Errorf("consensus store backend is nil")
	}
	return nil
}

func validateSignedBlock(signedBlock *types.SignedBlock, verify bool) (*types.Block, error) {
	if signedBlock == nil {
		return nil, fmt.Errorf("malformed signed block: signed block is nil")
	}
	if signedBlock.Block == nil {
		return nil, fmt.Errorf("malformed signed block: block is nil")
	}
	if signedBlock.Block.Body == nil {
		return nil, fmt.Errorf("malformed block: body is nil")
	}
	if verify && (signedBlock.Proof == nil || len(signedBlock.Proof.Proof) == 0) {
		return nil, &store.StoreError{
			Kind:    store.ErrAttestationSignatureMismatch,
			Message: "block proof missing",
		}
	}
	if signedBlock.Proof != nil && len(signedBlock.Proof.Proof) > types.ByteList512KiBMax {
		return nil, fmt.Errorf("block proof exceeds %d bytes", types.ByteList512KiBMax)
	}
	return signedBlock.Block, nil
}

// validateBlockAttestations enforces the wire-level prohibition on exact-duplicate
// AttestationData. The per-block cap on the distinct-data count lives in the state
// transition, where it binds every caller; split aggregates sharing one data entry
// are a legitimate, idempotently merged input and so are not rejected here.
func validateBlockAttestations(block *types.Block) error {
	seen := make(map[[32]byte]bool)
	for _, att := range block.Body.Attestations {
		if !validAttestationShape(att) {
			return fmt.Errorf("malformed block attestation")
		}

		dataRoot, err := att.Data.HashTreeRoot()
		if err != nil {
			return fmt.Errorf("hash attestation data: %w", err)
		}
		if seen[dataRoot] {
			return &store.StoreError{
				Kind:    store.ErrDuplicateAttestationData,
				Message: "block contains duplicate AttestationData",
			}
		}
		seen[dataRoot] = true
	}
	return nil
}

func validAttestationShape(att *types.AggregatedAttestation) bool {
	if att == nil || att.Data == nil {
		return false
	}
	return att.Data.Head != nil && att.Data.Target != nil && att.Data.Source != nil
}
