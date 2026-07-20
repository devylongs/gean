package testdriver

import (
	"bytes"
	"fmt"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/blockprocessor"
	"github.com/geanlabs/gean/internal/specfixtures"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

// mockedProofSentinel prefixes the placeholder aggregation proofs the upstream
// fixtures carry for weight-only vectors. A proof bearing it is not real crypto,
// so signature verification is skipped for it — mirroring the local spec-test
// harness so the same fixtures resolve identically through the HTTP driver.
var mockedProofSentinel = []byte("\x00MOCKED-AGGREGATION-PROOF\x00")

func carriesMockedProof(proof []byte) bool {
	return bytes.HasPrefix(proof, mockedProofSentinel)
}

// earliestAdmissibleInterval is the lowest store time at which a slot-N vote
// clears the future-admission check: admission allows
// slot*INTERVALS_PER_SLOT <= time + GOSSIP_DISPARITY_INTERVALS, so the vote is
// admissible one disparity interval before its slot starts. Advancing exactly to
// this interval — never to the slot start — keeps a following tick's promotion
// from being swallowed.
func earliestAdmissibleInterval(slot uint64) uint64 {
	start := slot * types.IntervalsPerSlot
	if start < types.GossipDisparityIntervals {
		return 0
	}
	return start - types.GossipDisparityIntervals
}

func (sess *Session) applyTick(step *specfixtures.ForkChoiceStep) error {
	cfg := sess.store.Config()
	if cfg == nil {
		return fmt.Errorf("tick step before config set")
	}

	var target uint64
	switch {
	case step.Time != nil:
		genesisMs := cfg.GenesisTime * 1000
		timestampMs := *step.Time * 1000
		if timestampMs >= genesisMs {
			target = (timestampMs - genesisMs) / types.MillisecondsPerInterval
		}
	case step.Interval != nil:
		target = *step.Interval
	default:
		return fmt.Errorf("tick step missing time and interval")
	}

	// Advance one interval at a time, mirroring leanSpec on_tick. Stepping is
	// load-bearing: jumping to the target would skip the intervening slot-boundary
	// intervals whose actions promote the new-vote pool into the known pool and
	// recompute the head. Promotion happens at interval 4 always, and at interval 0
	// when a proposal has landed — the latter only on the final interval, matching
	// the spec's should_signal_proposal gate.
	hasProposal := step.HasProposal != nil && *step.HasProposal
	for sess.store.Time() < target {
		next := sess.store.Time() + 1
		sess.store.SetTime(next)
		interval := next % types.IntervalsPerSlot
		signalProposal := hasProposal && next == target
		if interval == 4 || (interval == 0 && signalProposal) {
			sess.store.PromoteNewToKnown()
		}
		if interval == 0 || interval == 4 {
			sess.updateHeadFromKnown(sess.store.LatestJustified().Root)
		}
	}
	// A tick to at or behind the clock still pins store.time so a later time check
	// reads the fixture's value.
	if target < sess.store.Time() {
		sess.store.SetTime(target)
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

	// Advance the clock to this block's slot unless the fixture delivers it ahead
	// of the clock (tickToSlot=false). The clock gates attestation future-validation,
	// so it must reach the slot before subsequent attestations validate.
	if step.TickToSlot == nil || *step.TickToSlot {
		minTime := block.Slot * types.IntervalsPerSlot
		if sess.store.Time() < minTime {
			sess.store.SetTime(minTime)
		}
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

	// Seed the block's on-chain aggregated attestations into the known pool with
	// their participant sets so the votes carry fork-choice weight. Block import
	// alone stores the data weightless, which would mis-resolve weight-driven
	// reorgs. Only participants are read during head computation, so a non-empty
	// placeholder proof suffices.
	if block.Body != nil {
		for _, att := range block.Body.Attestations {
			if att == nil || att.Data == nil || types.BitlistCount(att.AggregationBits) == 0 {
				continue
			}
			dataRoot, err := att.Data.HashTreeRoot()
			if err != nil {
				continue
			}
			sess.store.KnownPayloads.Push(dataRoot, att.Data, &types.SingleMessageAggregate{
				Participants: att.AggregationBits,
				Proof:        []byte{0x01},
			})
		}
	}

	justifiedRoot := sess.store.LatestJustified().Root
	sess.updateHeadFromKnown(justifiedRoot)

	// Promote new payloads to known, then reflect the just-promoted votes into the
	// known tracker — the next head update re-derives known votes from the promoted
	// pool, so a vote gossiped as "new" would otherwise stay absent from the tracker.
	sess.store.PromoteNewToKnown()
	for vid, data := range sess.store.ExtractLatestKnownAttestations() {
		sess.fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
	}
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

	// For valid steps advance just enough to clear the future-admission check,
	// stopping one disparity interval before the slot start. Invalid steps keep the
	// current time so the rejection path still fires.
	if step.Valid {
		if minTime := earliestAdmissibleInterval(attData.Slot); sess.store.Time() < minTime {
			sess.store.SetTime(minTime)
		}
	}

	dataRoot, err := attData.HashTreeRoot()
	if err != nil {
		return err
	}
	signature, err := specfixtures.ParseHexBytes(step.Attestation.Signature)
	if err != nil {
		return fmt.Errorf("decode signature hex: %w", err)
	}

	// Data/bounds validation always runs so rejection fixtures exercise the
	// validator. The crypto check is skipped only for mocked placeholder proofs.
	// A non-nil error reports the step as not accepted; the simulator compares
	// that against the fixture's expected outcome.
	if err := attestation.ValidateAttestationData(sess.store, attData); err != nil {
		return err
	}
	if !carriesMockedProof(signature) {
		if err := attestation.VerifyGossipAttestation(sess.store, step.Attestation.ValidatorID, attData, dataRoot, signature); err != nil {
			return err
		}
	}

	participants := types.BitlistFromIndices([]uint64{step.Attestation.ValidatorID})
	sess.store.NewPayloads.Push(dataRoot, attData, &types.SingleMessageAggregate{Participants: participants})

	var sig [types.SignatureSize]byte
	copy(sig[:], signature)
	sess.store.AttestationSignatures.Insert(dataRoot, attData, step.Attestation.ValidatorID, sig)

	sess.fc.SetNewVote(step.Attestation.ValidatorID, attData.Head.Root, attData.Slot, attData)

	// Gossip lands in the new pool only; the head keeps reflecting the known pool
	// until a slot-boundary tick promotes these votes, so recompute from the known
	// pool here without promoting.
	sess.updateHeadFromKnown(sess.store.LatestJustified().Root)
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
		if minTime := earliestAdmissibleInterval(attData.Slot); sess.store.Time() < minTime {
			sess.store.SetTime(minTime)
		}
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

	if err := attestation.ValidateAttestationData(sess.store, attData); err != nil {
		return err
	}
	if !carriesMockedProof(proofData) {
		if err := attestation.VerifyAggregatedGossipAttestation(sess.store, attData, participants, proofData); err != nil {
			return err
		}
	}

	sess.store.NewPayloads.Push(dataRoot, attData, &types.SingleMessageAggregate{Participants: participants, Proof: proofData})

	for _, vid := range types.BitlistIndices(participants) {
		sess.fc.SetNewVote(vid, attData.Head.Root, attData.Slot, attData)
	}

	sess.updateHeadFromKnown(sess.store.LatestJustified().Root)
	return nil
}

// updateHeadFromKnown reflects the current known-pool votes into fork choice and
// recomputes the head, without promoting the new pool.
func (sess *Session) updateHeadFromKnown(justifiedRoot [32]byte) {
	for vid, data := range sess.store.ExtractLatestKnownAttestations() {
		sess.fc.SetKnownVote(vid, data.Head.Root, data.Slot, data)
	}
	sess.updateHead(justifiedRoot)
}

// updateHead recomputes the head and re-anchors the finalized checkpoint to the
// head's chain, mirroring the spec's update_head. Finalization tracks the head's
// chain unconditionally: a higher-finalized fork that loses head selection must
// not latch.
func (sess *Session) updateHead(justifiedRoot [32]byte) {
	headRoot := sess.fc.UpdateHead(justifiedRoot)
	sess.store.SetHead(headRoot)
	if derived := store.DeriveFinalizedFromHead(sess.store, headRoot); derived != nil {
		sess.store.SetLatestFinalized(derived)
	}
}

// refreshSafeTarget computes the safe target on demand. It mutates the protoarray
// scores with the supermajority threshold, so it must run after the head is
// already recomputed and stored for the step — never before, and only when a
// safe-target check needs the value.
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
