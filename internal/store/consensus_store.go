package store

import (
	"sync"
	"sync/atomic"

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

	// maxBlockSlot is the highest slot of any block header this store has
	// written: a high-water mark, not a live maximum over the table. It answers
	// MaxStoredBlockSlot, which the duty gate reads up to three times a slot and
	// which used to scan and SSZ-decode every header in TableBlockHeaders.
	//
	// It deliberately covers the same set the scan did — imported blocks and
	// blocks stored while pending — because the duty gate's network-stall
	// carve-out depends on it. Narrowing it to imported blocks only would let a
	// node that restarted holding a pending block at a near-current slot read
	// its own import lag as a network stall and resume duties on a stale head.
	//
	// The one behavioural difference from the scan: pruning the highest-slot
	// header no longer lowers the answer. That errs toward reporting the network
	// as alive, which is the direction that keeps the gate closed rather than
	// opening it onto a dead fork.
	maxBlockSlot atomic.Uint64
	// maxBlockSlotSeeded latches only once a seeding scan has completed without
	// error. A sync.Once would latch on failure too, and a scan that returns 0
	// because the read view could not be opened — or that stopped part-way
	// through the table — would permanently understate the mark. That is not
	// merely a bad gauge: an understated mark inflates the duty gate's computed
	// network lag, which can trip the network-stall carve-out and let the node
	// resume duties on a stale head.
	maxBlockSlotSeeded atomic.Bool
	maxBlockSlotSeedMu sync.Mutex
}

// ObserveStoredBlockSlot raises the stored-block high-water mark. Safe from any
// goroutine: block import runs on the dispatch loop, but pending-block writes
// and the test driver do not.
func (s *ConsensusStore) ObserveStoredBlockSlot(slot uint64) {
	if s == nil {
		return
	}
	for {
		current := s.maxBlockSlot.Load()
		if slot <= current {
			return
		}
		if s.maxBlockSlot.CompareAndSwap(current, slot) {
			return
		}
	}
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
