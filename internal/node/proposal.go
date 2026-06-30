package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/geanlabs/gean/internal/attestationproof"
	"github.com/geanlabs/gean/internal/blockbuilder"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/statetransition"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

type proposalDuty struct {
	slot        uint64
	validatorID uint64
}

type proposalResult struct {
	blockRoot   [32]byte
	signedBlock *types.SignedBlock
}

func (e *Engine) maybePropose(slot, validatorID uint64) {
	if e.Keys == nil {
		return
	}
	if e.Store.HeadSlot() >= slot {
		return
	}
	if e.DutyGate != nil && !e.DutyGate.Decide("block", slot, e.Store.HeadSlot(), e.Store.MaxStoredBlockSlot()) {
		metrics.IncBlocksSkippedLag()
		return
	}
	select {
	case e.ProposalCh <- proposalDuty{slot: slot, validatorID: validatorID}:
		metrics.SetProvingQueueDepth("proposal", len(e.ProposalCh))
	default:
		logger.Warn(logger.Validator, "proposal worker busy slot=%d", slot)
	}
}

func (e *Engine) runProposalWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case duty := <-e.ProposalCh:
			metrics.SetProvingQueueDepth("proposal", len(e.ProposalCh))
			// Duty may have gone stale while the worker proved an earlier slot.
			if head := e.Store.HeadSlot(); head >= duty.slot {
				logger.Warn(logger.Validator, "skipping stale proposal slot=%d head=%d", duty.slot, head)
				metrics.IncProofOperation("proposal", "skipped_stale")
				continue
			}
			deadline, cancel := context.WithTimeout(ctx, 3*types.MillisecondsPerInterval*time.Millisecond)
			if e.ProvingGate != nil && !e.ProvingGate.Acquire(deadline, true) {
				cancel()
				metrics.IncProofOperation("proposal", "canceled")
				logger.Error(logger.Validator, "proposal prover unavailable slot=%d", duty.slot)
				continue
			}
			result := e.buildProposal(duty.slot, duty.validatorID)
			if e.ProvingGate != nil {
				e.ProvingGate.Release(true)
			}
			cancel()
			if result == nil {
				continue
			}
			select {
			case e.ProposalResultCh <- result:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (e *Engine) buildProposal(slot, validatorID uint64) *proposalResult {
	logger.Info(logger.Validator, "proposing block slot=%d validator=%d", slot, validatorID)

	block, attSigProofs, err := e.produceBlockWithSignatures(slot, validatorID)
	if err != nil {
		// Head advanced past the target slot; skip as a slot-boundary miss (matches spec).
		var stale *statetransition.StateSlotIsNewerError
		if errors.As(err, &stale) {
			logger.Warn(logger.Validator, "skipping stale proposal slot=%d head=%d", slot, e.Store.HeadSlot())
			metrics.IncProofOperation("proposal", "skipped_stale")
			return nil
		}
		logger.Error(logger.Validator, "produce block failed: %v", err)
		return nil
	}

	signStart := time.Now()
	propKey := e.Keys.GetProposalKey(validatorID)
	if propKey == nil {
		logger.Error(logger.Validator, "proposal key not found for validator=%d", validatorID)
		return nil
	}
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		logger.Error(logger.Validator, "block root failed: %v", err)
		return nil
	}
	blockSig, err := propKey.Sign(uint32(slot), blockRoot)
	metrics.ObservePqSigSigningTime(time.Since(signStart).Seconds())
	if err != nil {
		logger.Error(logger.Validator, "sign block failed: %v", err)
		return nil
	}

	mergeStart := time.Now()
	proof, err := e.mergeBlockProof(block, attSigProofs, propKey, blockSig)
	metrics.ObserveProvingDuration("proposal", time.Since(mergeStart).Seconds())
	if err != nil {
		metrics.IncProofOperation("proposal", "error")
		logger.Error(logger.Validator, "merge block proof failed: %v", err)
		return nil
	}
	metrics.ObserveProofMergeComponents(len(attSigProofs) + 1)
	metrics.ObserveProofSize("type2", len(proof))
	return &proposalResult{
		blockRoot: blockRoot,
		signedBlock: &types.SignedBlock{
			Block: block,
			Proof: &types.MultiMessageAggregate{Proof: proof},
		},
	}
}

func (e *Engine) acceptProposal(ctx context.Context, result *proposalResult) {
	if result == nil || result.signedBlock == nil || result.signedBlock.Block == nil {
		return
	}
	block := result.signedBlock.Block
	// A proposal that outlived its slot is still the chain's best extension
	// unless the head moved past it; fork choice accepts late blocks, while
	// discarding one here can permanently halt a stalled chain.
	if e.Store.HeadSlot() >= block.Slot || e.Store.Head() != block.ParentRoot {
		metrics.IncProofOperation("proposal", "canceled")
		logger.Warn(logger.Validator, "discarding proposal slot=%d: head moved head_slot=%d head=0x%x parent=0x%x",
			block.Slot, e.Store.HeadSlot(), e.Store.Head(), block.ParentRoot)
		return
	}
	e.onBlock(result.signedBlock)
	if !e.Store.HasState(result.blockRoot) {
		metrics.IncProofOperation("proposal", "error")
		return
	}
	metrics.IncProofOperation("proposal", "success")

	if e.P2P != nil {
		publishCtx, cancel := context.WithTimeout(ctx, types.MillisecondsPerInterval*time.Millisecond)
		defer cancel()
		if err := e.P2P.PublishBlock(publishCtx, result.signedBlock); err != nil {
			logger.Error(logger.Network, "publish block failed: %v", err)
		}
	}

	attestationCount := 0
	if block.Body != nil {
		attestationCount = len(block.Body.Attestations)
	}
	logger.Info(logger.Validator, "proposed block slot=%d block_root=0x%x attestations=%d",
		block.Slot, result.blockRoot, attestationCount)
}

func (e *Engine) mergeBlockProof(
	block *types.Block,
	attestationProofs []*types.SingleMessageAggregate,
	proposerKey *xmss.ValidatorKeyPair,
	proposerSignature [types.SignatureSize]byte,
) ([]byte, error) {
	if block == nil || block.Body == nil || len(block.Body.Attestations) != len(attestationProofs) {
		return nil, fmt.Errorf("attestation proof count mismatch")
	}
	state := e.Store.GetState(block.ParentRoot)
	if state == nil {
		return nil, fmt.Errorf("parent state missing")
	}

	inputs := make([]xmss.Type1Input, 0, len(attestationProofs)+1)
	for i, proof := range attestationProofs {
		if proof == nil {
			return nil, fmt.Errorf("attestation proof %d missing", i)
		}
		keys := make([]xmss.CPubKey, 0, types.BitlistCount(proof.Participants))
		for _, index := range types.BitlistIndices(proof.Participants) {
			if index >= uint64(len(state.Validators)) || state.Validators[index] == nil {
				return nil, fmt.Errorf("attestation proof %d validator %d out of range", i, index)
			}
			key, err := e.Store.PubKeyCache.Get(state.Validators[index].AttestationPubkey)
			if err != nil {
				return nil, fmt.Errorf("attestation proof %d validator %d: %w", i, index, err)
			}
			keys = append(keys, key)
		}
		inputs = append(inputs, xmss.Type1Input{Pubkeys: keys, Proof: proof.Proof})
	}

	signature, err := xmss.ParseSignature(proposerSignature[:])
	if err != nil {
		return nil, err
	}
	defer xmss.FreeSignature(signature)
	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return nil, err
	}
	proposerProof, err := xmss.AggregateSignatures(
		[]xmss.CPubKey{proposerKey.PublicKey()},
		[]xmss.CSig{signature},
		blockRoot,
		uint32(block.Slot),
	)
	if err != nil {
		return nil, err
	}
	inputs = append(inputs, xmss.Type1Input{
		Pubkeys: []xmss.CPubKey{proposerKey.PublicKey()},
		Proof:   proposerProof,
	})
	return xmss.MergeType1Proofs(inputs)
}

func (e *Engine) produceBlockWithSignatures(slot, validatorIndex uint64) (*types.Block, []*types.SingleMessageAggregate, error) {
	buildStart := time.Now()
	defer func() { metrics.ObserveBlockBuildingTime(time.Since(buildStart).Seconds()) }()

	headRoot := e.Store.Head()
	headState := e.Store.GetState(headRoot)
	if headState == nil {
		metrics.IncBlockBuildingFailures()
		return nil, nil, fmt.Errorf("head state missing for slot %d", slot)
	}

	numValidators := headState.NumValidators()
	if !types.IsProposer(slot, validatorIndex, numValidators) {
		metrics.IncBlockBuildingFailures()
		return nil, nil, fmt.Errorf("validator %d not proposer for slot %d", validatorIndex, slot)
	}

	knownBlockRoots, err := e.Store.BlockRoots()
	if err != nil {
		metrics.IncBlockBuildingFailures()
		return nil, nil, fmt.Errorf("load block roots: %w", err)
	}

	result, err := blockbuilder.Build(blockbuilder.Input{
		HeadState:       headState,
		Slot:            slot,
		ProposerIndex:   validatorIndex,
		ParentRoot:      headRoot,
		KnownBlockRoots: knownBlockRoots,
		Payloads:        payloadsFromEntries(e.Store.KnownPayloads.Entries()),
		ProofMerger:     attestationproof.NewMerger(e.Store.PubKeyCache),
	})
	if err != nil {
		metrics.IncBlockBuildingFailures()
		return nil, nil, err
	}
	for _, payloadErr := range result.PayloadErrors {
		if blockbuilder.IsExpectedSkip(payloadErr.Err) {
			continue
		}
		logger.Warn(logger.Validator, "block payload issue root=0x%x: %v", payloadErr.DataRoot, payloadErr.Err)
	}

	metrics.IncBlockBuildingSuccess()
	if result.Block != nil && result.Block.Body != nil {
		metrics.ObserveBlockAggregatedPayloads(len(result.Block.Body.Attestations))
	}
	return result.Block, result.AttestationProofs, nil
}

func payloadsFromEntries(entries map[[32]byte]*store.PayloadEntry) []blockbuilder.AttestationPayload {
	payloads := make([]blockbuilder.AttestationPayload, 0, len(entries))
	for dataRoot, entry := range entries {
		payload := blockbuilder.AttestationPayload{DataRoot: dataRoot}
		if entry != nil {
			payload.Data = entry.Data
			payload.Proofs = make([]*types.SingleMessageAggregate, len(entry.Proofs))
			copy(payload.Proofs, entry.Proofs)
		}
		payloads = append(payloads, payload)
	}
	return payloads
}
