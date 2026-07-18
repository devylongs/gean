package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/attestation"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

func (e *Engine) produceAttestations(slot uint64) {
	if e.Keys == nil {
		return
	}

	if e.DutyGate != nil && !e.DutyGate.Decide("attestation", slot, e.Store.HeadSlot(), e.networkSeenSlot()) {
		metrics.IncAttestationsSkippedLag()
		return
	}

	attData := attestation.ProduceAttestationData(e.Store, slot)
	if attData == nil {
		return
	}

	for _, vid := range e.Keys.ValidatorIDs() {
		prodStart := time.Now()

		sStart := time.Now()
		sig, err := e.Keys.SignAttestation(vid, attData)
		metrics.ObservePqSigSigningTime(time.Since(sStart).Seconds())
		if err != nil {
			logger.Error(logger.Validator, "sign attestation failed validator=%d: %v", vid, err)
			continue
		}

		signedAtt := &types.SignedAttestation{
			ValidatorID: vid,
			Data:        attData,
			Signature:   sig,
		}

		logger.Info(logger.Validator, "produced attestation slot=%d validator=%d", slot, vid)

		if e.AggCtl != nil && e.AggCtl.Get() {
			dataRoot, err := attData.HashTreeRoot()
			if err != nil {
				logger.Error(logger.Validator, "attestation root failed validator=%d: %v", vid, err)
				continue
			}
			sigHandle, parseErr := xmss.ParseSignature(sig[:])
			e.Store.AttestationSignatures.InsertWithHandle(dataRoot, attData, vid, sig, sigHandle, parseErr)
		}

		if e.P2P != nil {
			if err := e.P2P.PublishAttestation(context.Background(), signedAtt, e.CommitteeCount); err != nil {
				logger.Error(logger.Network, "publish attestation failed validator=%d: %v", vid, err)
			} else {
				logger.Info(logger.Network, "published attestation to network slot=%d validator=%d", slot, vid)
			}
		}

		metrics.ObserveAttestationsProductionTime(time.Since(prodStart).Seconds())
	}
}

func (e *Engine) getOurProposer(slot uint64) (uint64, bool) {
	if e.Keys == nil {
		return 0, false
	}
	headState := e.Store.GetState(e.Store.Head())
	if headState == nil {
		return 0, false
	}
	numValidators := headState.NumValidators()

	for _, vid := range e.Keys.ValidatorIDs() {
		if types.IsProposer(slot, vid, numValidators) {
			return vid, true
		}
	}
	return 0, false
}
