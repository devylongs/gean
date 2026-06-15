package blockprocessor

import (
	"fmt"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func OnBlock(s *store.ConsensusStore, signedBlock *types.SignedBlock) error {
	return onBlockCore(s, signedBlock, true)
}

func OnBlockWithoutVerification(s *store.ConsensusStore, signedBlock *types.SignedBlock) error {
	return onBlockCore(s, signedBlock, false)
}

func onBlockCore(s *store.ConsensusStore, signedBlock *types.SignedBlock, verify bool) error {
	start := time.Now()
	if err := validateStore(s); err != nil {
		return err
	}

	block, err := validateSignedBlock(signedBlock, verify)
	if err != nil {
		return err
	}

	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("compute block root: %w", err)
	}
	if s.HasState(blockRoot) {
		return nil
	}

	parentState := s.GetState(block.ParentRoot)
	if parentState == nil {
		return &store.StoreError{
			Kind:    store.ErrMissingParentState,
			Message: fmt.Sprintf("parent state not found for slot %d, missing block %x", block.Slot, block.ParentRoot),
		}
	}

	if err := validateBlockAttestations(block); err != nil {
		return err
	}

	if verify {
		verifyStart := time.Now()
		err := verifyBlockSignatures(s, signedBlock, parentState)
		metrics.ObserveBlockSignatureVerificationTime(time.Since(verifyStart).Seconds())
		if err != nil {
			return err
		}
	}

	stfStart := time.Now()
	postState, err := transitionState(parentState, block)
	if err != nil {
		return &store.StoreError{Kind: store.ErrStateTransitionFailed, Message: fmt.Sprintf("state transition: %v", err)}
	}
	metrics.ObserveSTFTime(time.Since(stfStart).Seconds())

	postState.LatestBlockHeader.StateRoot = block.StateRoot
	if err := persistBlock(s, blockRoot, signedBlock, postState); err != nil {
		return err
	}
	importBlockAttestations(s, signedBlock)

	logBlockProcessed(s, block, blockRoot, time.Since(start))
	return nil
}

func logBlockProcessed(s *store.ConsensusStore, block *types.Block, blockRoot [32]byte, elapsed time.Duration) {
	attCount := 0
	if block.Body != nil {
		attCount = len(block.Body.Attestations)
	}

	logger.Info(logger.Chain, "block slot=%d block_root=0x%x parent_root=0x%x proposer=%d attestations=%d justified_slot=%d finalized_slot=%d proc_time=%s",
		block.Slot, blockRoot, block.ParentRoot, block.ProposerIndex, attCount,
		s.LatestJustified().Slot, s.LatestFinalized().Slot,
		elapsed.Round(time.Millisecond))
	metrics.ObserveBlockProcessingTime(elapsed.Seconds())
}
