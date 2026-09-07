package store

import (
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/xmss"
)

// Buffer bounds. These are stall insurance, not an operating limit: while
// finalization advances, PruneOnFinalization clears all three pools every few
// slots and none of them comes close. They matter when finalization stops,
// because that is also when the only pruning path for these pools stops.
const (
	// aggregatedPayloadCap counts proofs, not attestation data roots. A proof
	// may reach MaxProofSize (512 KiB), so the cap is deliberately modest;
	// with one aggregate per committee per slot it still holds tens of slots
	// of history at devnet-5's eight committees.
	aggregatedPayloadCap = 512
	// newPayloadCap is smaller because the interval-4 promotion drains this
	// buffer into KnownPayloads every slot.
	newPayloadCap = 64
	// gossipSignatureCap counts individual signatures across all data roots.
	// Each entry carries a SignatureSize (1208 byte) array, so 8192 of them is
	// about 10 MiB: sixteen slots of backlog at devnet-5's 512 validators, and
	// far more than a healthy node ever holds between prunes.
	gossipSignatureCap = 8192
)

type ConsensusStore struct {
	Backend               storage.Backend
	NewPayloads           *PayloadBuffer
	KnownPayloads         *PayloadBuffer
	AttestationSignatures AttestationSignatureMap
	PubKeyCache           *xmss.PubKeyCache
}

func NewConsensusStore(backend storage.Backend) *ConsensusStore {
	return &ConsensusStore{
		Backend:               backend,
		NewPayloads:           NewPayloadBuffer(newPayloadCap),
		KnownPayloads:         NewPayloadBuffer(aggregatedPayloadCap),
		AttestationSignatures: NewAttestationSignatureMap(gossipSignatureCap),
		PubKeyCache:           xmss.NewPubKeyCache(),
	}
}
