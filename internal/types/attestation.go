package types

type AttestationData struct {
	Slot   uint64      `json:"slot"`
	Head   *Checkpoint `json:"head"`
	Target *Checkpoint `json:"target"`
	Source *Checkpoint `json:"source"`
}

type Attestation struct {
	ValidatorID uint64           `json:"validator_id"`
	Data        *AttestationData `json:"data"`
}

type SignedAttestation struct {
	ValidatorID uint64              `json:"validator_id"`
	Data        *AttestationData    `json:"data"`
	Signature   [SignatureSize]byte `json:"signature" ssz-size:"2536"`
}

type AggregatedAttestation struct {
	AggregationBits []byte           `json:"aggregation_bits" ssz:"bitlist" ssz-max:"4096"`
	Data            *AttestationData `json:"data"`
}

type SingleMessageAggregate struct {
	Participants []byte `json:"participants" ssz:"bitlist" ssz-max:"4096"`
	Proof        []byte `json:"proof" ssz-max:"524288"`
}

type SignedAggregatedAttestation struct {
	Data  *AttestationData        `json:"data"`
	Proof *SingleMessageAggregate `json:"proof"`
}
