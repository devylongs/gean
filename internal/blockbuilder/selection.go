package blockbuilder

import (
	"github.com/geanlabs/gean/internal/attestationproof"
	"github.com/geanlabs/gean/internal/types"
)

type proofMerger = attestationproof.MergeProvider

func selectPayloadAttestation(
	payload AttestationPayload,
	state *types.State,
	merger proofMerger,
) (*types.AggregatedAttestation, *types.SingleMessageAggregate, bool, error) {
	return attestationproof.Select(payload.Data, payload.Proofs, state, merger)
}
