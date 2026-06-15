package testdriver

import (
	"fmt"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/blockprocessor"
	"github.com/geanlabs/gean/internal/specfixtures"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func (sess *Session) applyTick(step *specfixtures.ForkChoiceStep) error {
	cfg := sess.store.Config()
	if cfg == nil {
		return fmt.Errorf("tick step before config set")
	}
	genesisMs := cfg.GenesisTime * 1000

	switch {
	case step.Time != nil:
		timestampMs := *step.Time * 1000
		if timestampMs < genesisMs {
			sess.store.SetTime(0)
		} else {
			sess.store.SetTime((timestampMs - genesisMs) / types.MillisecondsPerInterval)
		}
	case step.Interval != nil:
		sess.store.SetTime(*step.Interval)
	default:
		return fmt.Errorf("tick step missing time and interval")
	}
	return nil
}

func (sess *Session) applyBlock(step *specfixtures.ForkChoiceStep) error {
	if step.Block == nil {
		return fmt.Errorf("block step missing block payload")
	}
	block, err := step.Block.ToBlock()
	if err != nil {
		return err
	}

	signedBlock := &types.SignedBlock{Block: block, Proof: &types.MultiMessageAggregate{}}

	minTime := block.Slot * types.IntervalsPerSlot
	if sess.store.Time() < minTime {
		sess.store.SetTime(minTime)
	}

	if err := blockprocessor.OnBlockWithoutVerification(sess.store, signedBlock); err != nil {
		return err
	}

	blockRoot, err := block.HashTreeRoot()
	if err != nil {
		return err
	}
	if step.Block.BlockRootLabel != "" {
		sess.labelRoots[step.Block.BlockRootLabel] = blockRoot
	}
	sess.fc.OnBlock(block.Slot, blockRoot, block.ParentRoot)

	attestations := sess.store.ExtractLatestKnownAttestations()
	for vid, data := range attestations {
		sess.fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
	}
	justifiedRoot := sess.store.LatestJustified().Root
	sess.setCanonicalHead(justifiedRoot)
	sess.store.PromoteNewToKnown()
	return nil
}

func (sess *Session) applyAttestation(step *specfixtures.ForkChoiceStep) error {
	if step.Attestation == nil {
		return fmt.Errorf("attestation step missing attestation payload")
	}
	attData, err := step.Attestation.Data.ToAttestationData()
	if err != nil {
		return err
	}

	if step.Valid {
		minTime := attData.Slot * types.IntervalsPerSlot
		if sess.store.Time() < minTime {
			sess.store.SetTime(minTime)
		}
	}
	if err := attestation.ValidateAttestationData(sess.store, attData); err != nil {
		return err
	}

	dataRoot, err := attData.HashTreeRoot()
	if err != nil {
		return err
	}
	if err := sess.verifyGossipAttestation(step.Attestation.ValidatorID, attData, dataRoot, step.Attestation.Signature); err != nil {
		return err
	}

	participants := types.BitlistFromIndices([]uint64{step.Attestation.ValidatorID})
	proof := &types.SingleMessageAggregate{Participants: participants}
	sess.store.NewPayloads.Push(dataRoot, attData, proof)

	sess.fc.SetNewVote(step.Attestation.ValidatorID, attData.Head.Root, attData.Slot, attData)

	sess.promoteVotesAndUpdateHead()
	return nil
}

func (sess *Session) applyAggregatedAttestation(step *specfixtures.ForkChoiceStep) error {
	if step.Attestation == nil {
		return fmt.Errorf("gossipAggregatedAttestation step missing attestation payload")
	}
	attData, err := step.Attestation.Data.ToAttestationData()
	if err != nil {
		return err
	}

	if step.Valid {
		minTime := attData.Slot * types.IntervalsPerSlot
		if sess.store.Time() < minTime {
			sess.store.SetTime(minTime)
		}
	}
	if err := attestation.ValidateAttestationData(sess.store, attData); err != nil {
		return err
	}

	var participants []byte
	var proofData []byte
	if step.Attestation.Proof != nil {
		if participants, err = specfixtures.ParseBoolBitlist(step.Attestation.Proof.Participants.Data); err != nil {
			return fmt.Errorf("proof.participants: %w", err)
		}
		if proofData, err = specfixtures.ParseHexBytes(step.Attestation.Proof.Proof.Data); err != nil {
			return fmt.Errorf("proof.proofData: %w", err)
		}
	}
	dataRoot, err := attData.HashTreeRoot()
	if err != nil {
		return err
	}
	if err := attestation.VerifyAggregatedGossipAttestation(sess.store, attData, participants, proofData); err != nil {
		return err
	}

	proof := &types.SingleMessageAggregate{Participants: participants, Proof: proofData}
	sess.store.NewPayloads.Push(dataRoot, attData, proof)

	for _, vid := range types.BitlistIndices(participants) {
		sess.fc.SetNewVote(vid, attData.Head.Root, attData.Slot, attData)
	}

	sess.promoteVotesAndUpdateHead()
	return nil
}

func (sess *Session) verifyGossipAttestation(validatorID uint64, attData *types.AttestationData, dataRoot [32]byte, signatureHex string) error {
	sigBytes, err := specfixtures.ParseHexBytes(signatureHex)
	if err != nil {
		return fmt.Errorf("decode signature hex: %w", err)
	}
	return attestation.VerifyGossipAttestation(sess.store, validatorID, attData, dataRoot, sigBytes)
}

func (sess *Session) promoteVotesAndUpdateHead() {
	sess.store.PromoteNewToKnown()
	knownAtts := sess.store.ExtractLatestKnownAttestations()
	for vid, data := range knownAtts {
		sess.fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
	}
	justifiedRoot := sess.store.LatestJustified().Root
	sess.setCanonicalHead(justifiedRoot)
}

// setCanonicalHead recomputes the head and re-anchors the finalized checkpoint to
// the head's chain, mirroring the spec's update_head. Finalization is derived here
// rather than during block import, matching the live node's head selection.
func (sess *Session) setCanonicalHead(justifiedRoot [32]byte) {
	headRoot := sess.fc.UpdateHead(justifiedRoot)
	sess.store.SetHead(headRoot)

	derived := store.DeriveFinalizedFromHead(sess.store, headRoot)
	if derived == nil {
		return
	}
	if current := sess.store.LatestFinalized(); current != nil && derived.Slot <= current.Slot {
		return
	}
	sess.store.SetLatestFinalized(derived)
}

func (sess *Session) refreshSafeTarget() {
	headState := sess.store.GetState(sess.store.Head())
	if headState == nil {
		return
	}
	justifiedRoot := sess.store.LatestJustified().Root
	numValidators := uint64(len(headState.Validators))
	safeTarget := sess.fc.UpdateSafeTarget(justifiedRoot, numValidators)
	sess.store.SetSafeTarget(safeTarget)
}

func (sess *Session) loadSnapshot() driverSnapshot {
	headRoot := sess.store.Head()
	headSlot := uint64(0)
	if hdr := sess.store.GetBlockHeader(headRoot); hdr != nil {
		headSlot = hdr.Slot
	}
	justified := sess.store.LatestJustified()
	finalized := sess.store.LatestFinalized()
	safeTarget := sess.store.SafeTarget()
	return driverSnapshot{
		HeadSlot:            headSlot,
		HeadRoot:            fmt.Sprintf("0x%x", headRoot),
		Time:                sess.store.Time(),
		JustifiedCheckpoint: driverCheckpoint{Slot: justified.Slot, Root: fmt.Sprintf("0x%x", justified.Root)},
		FinalizedCheckpoint: driverCheckpoint{Slot: finalized.Slot, Root: fmt.Sprintf("0x%x", finalized.Root)},
		SafeTarget:          fmt.Sprintf("0x%x", safeTarget),
	}
}
