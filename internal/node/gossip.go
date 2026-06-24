package node

import (
	"time"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
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
	metrics.ObservePqSigVerificationTime(time.Since(verifyStart).Seconds())
	e.ShadowCosts.DelayVerify()
	if err != nil {
		metrics.IncPqSigAttestationSigsInvalid()
		metrics.IncAttestationsInvalid()
		return
	}
	metrics.IncPqSigAttestationSigsValid()
	metrics.IncAttestationsValid(1)

	sigHandle, parseErr := xmss.ParseSignature(att.Signature[:])

	logger.Info(logger.Gossip, "attestation verified: validator=%d slot=%d dataRoot=%x", att.ValidatorID, att.Data.Slot, dataRoot)
	e.Store.AttestationSignatures.InsertWithHandle(dataRoot, att.Data, att.ValidatorID, att.Signature, sigHandle, parseErr)
	success = true
}

func (e *Engine) onGossipAggregatedAttestation(agg *types.SignedAggregatedAttestation) {
	if agg == nil {
		return
	}

	if err := attestation.ValidateAttestationData(e.Store, agg.Data); err != nil {
		return
	}

	if agg.Proof == nil || len(agg.Proof.ProofData) == 0 {
		return
	}
	verifyStart := time.Now()
	err := attestation.VerifyAggregatedGossipAttestation(e.Store, agg.Data, agg.Proof.Participants, agg.Proof.ProofData)
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
