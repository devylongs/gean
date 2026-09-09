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
	// aggregatedPayloadCap counts proofs, not entries. Block import calls
	// PushData for every attestation it sees, which adds a data-only entry
	// carrying no proof bytes, and those do not count against this cap at all —
	// on devnet-5 most of roughly 2,900 known entries were of that kind. That is
	// deliberate: an entry is an AttestationData, a few hundred bytes, while a
	// proof may reach MaxProofSize (512 KiB), so weighting by proofs is what
	// bounds memory. Entry growth is bounded by pruning instead, including
	// PruneStaleAttestationPools while finalization is stalled.
	//
	// At devnet-5's eight committees, with aggregates arriving from each and
	// held until finalization, the live proof count is tens rather than
	// hundreds; 512 leaves an order of magnitude of headroom.
	aggregatedPayloadCap = 512
	// newPayloadCap is smaller because the interval-4 promotion drains this
	// buffer into KnownPayloads every slot.
	newPayloadCap = 64
	// gossipSignatureCap counts individual signatures across all data roots.
	// Each carries a SignatureSize (1208 byte) array, so 8192 is about 10 MiB.
	// A devnet-5 aggregator was observed holding roughly 2,300 between prunes,
	// so this is around three times the measured working set: high enough that
	// eviction never competes with normal aggregation, low enough to bound a
	// stall. Evicting a root that was about to be aggregated would drop live
	// votes silently, so the headroom matters more than the tightness.
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
