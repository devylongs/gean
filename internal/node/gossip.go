package node

import (
	"time"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func (e *Engine) onGossipAttestation(att *types.SignedAttestation) {
	if e.AggCtl == nil || !e.AggCtl.Get() || att == nil {
		return
	}

	start := time.Now()
	success := false
	defer func() {
		if success {
			metrics.ObserveAttestationValidationTime(time.Since(start).Seconds())
		}
	}()

	if err := attestation.ValidateAttestationData(e.Store, att.Data); err != nil {
		if se, ok := err.(*store.StoreError); ok && se.Kind == store.ErrUnknownHeadBlock && att.Data.Head != nil {
			added, dropped := e.PendingAttestations.Add(att.Data.Head.Root, att)
			if added {
				select {
				case e.FetchRootCh <- att.Data.Head.Root:
				default:
				}
			}
			if dropped > 0 {
				metrics.IncAttestationsBufferEvicted(dropped)
			}
		}
		return
	}

	dataRoot, err := att.Data.HashTreeRoot()
	if err != nil {
		logger.Error(logger.Gossip, "attestation root failed validator=%d: %v", att.ValidatorID, err)
		return
	}

	metrics.IncPqSigAttestationSigsTotal()
	verifyStart := time.Now()
	err = attestation.VerifyGossipAttestation(e.Store, att.ValidatorID, att.Data, dataRoot, att.Signature[:])
	e.Shadow.SleepVerify()
	metrics.ObservePqSigVerificationTime(time.Since(verifyStart).Seconds())
	if err != nil {
		metrics.IncPqSigAttestationSigsInvalid()
		metrics.IncAttestationsInvalid()
		return
	}
	metrics.IncPqSigAttestationSigsValid()
	metrics.IncAttestationsValid(1)

	logger.Info(logger.Gossip, "attestation verified: validator=%d slot=%d dataRoot=%x", att.ValidatorID, att.Data.Slot, dataRoot)
	e.Store.AttestationSignatures.Insert(dataRoot, att.Data, att.ValidatorID, att.Signature)
	success = true

	// Nudge the dispatch loop to consider aggregating early now that another vote
	// is in. The signal is best-effort and coalescing; the loop owns the actual
	// timing and quorum decision.
	select {
	case e.EarlyAggregateCh <- struct{}{}:
	default:
	}
}

func (e *Engine) onGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation) {
	if agg == nil {
		return
	}

	if err := attestation.ValidateAttestationData(e.Store, agg.Data); err != nil {
		return
	}

	if agg.Proof == nil || len(agg.Proof.Proof) == 0 {
		return
	}
	verifyStart := time.Now()
	err := attestation.VerifyAggregatedGossipAttestation(e.Store, agg.Data, agg.Proof.Participants, agg.Proof.Proof)
	e.Shadow.SleepVerifyAggregated(int(types.BitlistCount(agg.Proof.Participants)))
	metrics.ObservePqSigAggVerificationTime(time.Since(verifyStart).Seconds())
	if err != nil {
		metrics.IncPqSigAggregatedInvalid()
		logger.Error(logger.Signature, "aggregated attestation verification failed: %v", err)
		return
	}
	metrics.IncPqSigAggregatedValid()

	dataRoot, err := agg.Data.HashTreeRoot()
	if err != nil {
		logger.Error(logger.Signature, "aggregated attestation root failed: %v", err)
		return
	}
	e.Store.NewPayloads.Push(dataRoot, agg.Data, agg.Proof)
}
