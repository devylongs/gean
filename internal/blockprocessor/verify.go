package blockprocessor

import (
	"fmt"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func verifyBlockSignatures(
	s *store.ConsensusStore,
	signedBlock *types.SignedBlock,
	state *types.State,
) error {
	block, err := validateSignedBlock(signedBlock, true)
	if err != nil {
		return err
	}
	if state == nil {
		return &store.StoreError{Kind: store.ErrMissingParentState, Message: "parent state missing"}
	}
	if s.PubKeyCache == nil {
		return &store.StoreError{Kind: store.ErrPubkeyDecodingFailed, Message: "pubkey cache missing"}
	}

	pubkeys := make([][]xmss.CPubKey, 0, len(block.Body.Attestations)+1)
	bindings := make([]xmss.MessageBinding, 0, len(block.Body.Attestations)+1)
	for i, att := range block.Body.Attestations {
		indices := types.BitlistIndices(att.AggregationBits)
		if len(indices) == 0 {
			return &store.StoreError{Kind: store.ErrParticipantsMismatch, Message: fmt.Sprintf("attestation %d has no participants", i)}
		}
		keys := make([]xmss.CPubKey, 0, len(indices))
		for _, index := range indices {
			validator, err := validatorAt(state, index)
			if err != nil {
				return err
			}
			key, err := s.PubKeyCache.Get(validator.AttestationPubkey)
			if err != nil {
				return &store.StoreError{Kind: store.ErrPubkeyDecodingFailed, Message: fmt.Sprintf("validator %d: %v", index, err)}
			}
			keys = append(keys, key)
		}
		root, err := att.Data.HashTreeRoot()
		if err != nil {
			return fmt.Errorf("hash attestation %d data: %w", i, err)
		}
		slot, err := slot32(att.Data.Slot)
		if err != nil {
			return &store.StoreError{Kind: store.ErrSignatureDecodingFailed, Message: fmt.Sprintf("attestation %d slot: %v", i, err)}
		}
		pubkeys = append(pubkeys, keys)
		bindings = append(bindings, xmss.MessageBinding{Message: root, Slot: slot})
	}

	proposer, err := validatorAt(state, block.ProposerIndex)
	if err != nil {
		return err
	}
	proposerKey, err := s.PubKeyCache.Get(proposer.ProposalPubkey)
	if err != nil {
		return &store.StoreError{Kind: store.ErrPubkeyDecodingFailed, Message: fmt.Sprintf("proposer %d: %v", block.ProposerIndex, err)}
	}
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("compute block root: %w", err)
	}
	slot, err := slot32(block.Slot)
	if err != nil {
		return &store.StoreError{Kind: store.ErrProposerSignatureDecodingFailed, Message: fmt.Sprintf("proposer slot: %v", err)}
	}
	pubkeys = append(pubkeys, []xmss.CPubKey{proposerKey})
	bindings = append(bindings, xmss.MessageBinding{Message: blockRoot, Slot: slot})

	if err := xmss.VerifyType2Proof(signedBlock.Proof.Proof, pubkeys, bindings); err != nil {
		return &store.StoreError{Kind: store.ErrAggregateVerificationFailed, Message: fmt.Sprintf("block proof: %v", err)}
	}
	return nil
}
