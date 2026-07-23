package blockbuilder

import (
	"fmt"

	"github.com/geanlabs/gean/internal/types"
)

type planResult struct {
	attestations  []*types.AggregatedAttestation
	proofs        []*types.SingleMessageAggregate
	postState     *types.State
	payloadErrors []PayloadError
}

type planner struct {
	headState     *types.State
	slot          uint64
	proposerIndex uint64
	parentRoot    [32]byte
	knownRoots    KnownRoots
	proofMerger   proofMerger
	payloads      []AttestationPayload
	processed     map[[32]byte]bool
	attestations  []*types.AggregatedAttestation
	proofs        []*types.SingleMessageAggregate
	state         *types.State
	progress      progressSnapshot
	payloadErrors []PayloadError
	full          bool
}

func planAttestations(input Input) (planResult, error) {
	workingState, err := transitionBlock(
		input.HeadState,
		input.Slot,
		newBlock(input.Slot, input.ProposerIndex, input.ParentRoot, nil),
	)
	if err != nil {
		return planResult{}, err
	}
	if len(input.Payloads) == 0 {
		return planResult{postState: workingState}, nil
	}

	sorted, payloadErrors := sortedPayloads(input.Payloads)
	planner := &planner{
		headState:     input.HeadState,
		slot:          input.Slot,
		proposerIndex: input.ProposerIndex,
		parentRoot:    input.ParentRoot,
		knownRoots:    input.KnownBlockRoots,
		proofMerger:   input.ProofMerger,
		payloads:      sorted,
		processed:     make(map[[32]byte]bool),
		state:         workingState,
		progress:      captureProgress(workingState),
		payloadErrors: payloadErrors,
	}
	if err := planner.run(); err != nil {
		return planResult{}, err
	}
	return planner.result(), nil
}

func (p *planner) run() error {
	for len(p.attestations) < int(types.MaxAttestationsData) {
		if !p.runRound() {
			return nil
		}
		changed, err := p.transition()
		if err != nil {
			return err
		}
		if len(p.attestations) >= int(types.MaxAttestationsData) {
			p.full = true
			return nil
		}
		if !changed {
			return nil
		}
	}
	p.full = true
	return nil
}

func (p *planner) runRound() bool {
	added := false
	for _, payload := range p.payloads {
		if len(p.attestations) >= int(types.MaxAttestationsData) {
			return added
		}
		if p.tryPayload(payload) {
			added = true
		}
	}
	return added
}

func (p *planner) tryPayload(payload AttestationPayload) bool {
	if p.processed[payload.DataRoot] {
		return false
	}
	if payloadBuildIssue(p.state, p.knownRoots, payload) != nil {
		return false
	}

	p.processed[payload.DataRoot] = true
	att, sig, ok, selectErr := selectPayloadAttestation(payload, p.state, p.proofMerger)
	if selectErr != nil {
		p.recordPayloadError(payload.DataRoot, selectErr)
	}
	if !ok {
		return false
	}

	p.attestations = append(p.attestations, att)
	p.proofs = append(p.proofs, sig)
	return true
}

func (p *planner) transition() (bool, error) {
	candidate := newBlock(p.slot, p.proposerIndex, p.parentRoot, p.attestations)
	trialState, err := transitionBlock(p.headState, p.slot, candidate)
	if err != nil {
		return false, fmt.Errorf("trial transition: %w", err)
	}

	nextProgress := captureProgress(trialState)
	changed := progressChanged(p.progress, nextProgress)
	p.state = trialState
	if changed {
		p.progress = nextProgress
	}
	return changed, nil
}

func (p *planner) recordPayloadError(dataRoot [32]byte, err error) {
	p.payloadErrors = append(p.payloadErrors, PayloadError{
		DataRoot: dataRoot,
		Err:      err,
	})
}

func (p *planner) result() planResult {
	p.recordSkippedPayloads()
	return planResult{
		attestations:  p.attestations,
		proofs:        p.proofs,
		postState:     p.state,
		payloadErrors: p.payloadErrors,
	}
}

func (p *planner) recordSkippedPayloads() {
	if p.full {
		return
	}
	for _, payload := range p.payloads {
		if p.processed[payload.DataRoot] {
			continue
		}
		if err := payloadBuildIssue(p.state, p.knownRoots, payload); err != nil {
			p.recordPayloadError(payload.DataRoot, err)
		}
	}
}
