package store

import (
	"sync"

	"github.com/geanlabs/gean/internal/types"
)

// AttestationSignatureEntry holds only the serialized signature, never a parsed
// XMSS handle. The aggregation worker runs asynchronously on a snapshot of this
// map, so a handle cached here could be freed by a prune while the worker still
// held the snapshot's copy of the pointer — a use-after-free that segfaulted the
// prover. The worker parses its own handle from these bytes and owns its lifetime.
type AttestationSignatureEntry struct {
	ValidatorID uint64
	Signature   [types.SignatureSize]byte
}

type AttestationDataEntry struct {
	Data       *types.AttestationData
	Signatures []AttestationSignatureEntry
}

type AttestationSignatureMap struct {
	mu    sync.Mutex
	data  map[[32]byte]*AttestationDataEntry
	order [][32]byte
	total int
	// capacity bounds total signatures held, not data roots. Zero disables the
	// bound, which is only useful in tests that assert prune behaviour.
	capacity int
}

func NewAttestationSignatureMap(capacity int) AttestationSignatureMap {
	return AttestationSignatureMap{
		data:     make(map[[32]byte]*AttestationDataEntry),
		capacity: capacity,
	}
}

func (m *AttestationSignatureMap) Insert(dataRoot [32]byte, data *types.AttestationData, validatorID uint64, sig [types.SignatureSize]byte) {
	if data == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[[32]byte]*AttestationDataEntry)
	}
	entry, ok := m.data[dataRoot]
	if !ok {
		entry = &AttestationDataEntry{Data: copyAttestationData(data)}
		m.data[dataRoot] = entry
		m.order = append(m.order, dataRoot)
	}
	// One vote per validator per attestation data. Gossip meshes deliver the
	// same attestation more than once, and the pending-attestation replay path
	// re-enters the handler for buffered votes, so without this the same
	// signature is stored repeatedly: wasted memory, and a vote count that
	// overstates how many distinct validators have actually voted.
	for _, existing := range entry.Signatures {
		if existing.ValidatorID == validatorID {
			return
		}
	}
	entry.Signatures = append(entry.Signatures, AttestationSignatureEntry{
		ValidatorID: validatorID,
		Signature:   sig,
	})
	m.total++
	m.evictLocked()
}

// evictLocked drops whole data roots oldest-first until the signature count is
// back inside capacity. Evicting by root rather than by individual signature
// keeps a surviving root's votes complete, which is what an aggregate needs.
func (m *AttestationSignatureMap) evictLocked() {
	if m.capacity <= 0 {
		return
	}
	for m.total > m.capacity && len(m.order) > 0 {
		oldest := m.order[0]
		m.order = m.order[1:]
		if entry, ok := m.data[oldest]; ok {
			m.total -= len(entry.Signatures)
			delete(m.data, oldest)
		}
	}
}

// dropRootLocked removes a root from both the map and the insertion order.
func (m *AttestationSignatureMap) dropRootLocked(root [32]byte) {
	delete(m.data, root)
	for i, r := range m.order {
		if r == root {
			m.order = append(m.order[:i], m.order[i+1:]...)
			return
		}
	}
}

// Has reports whether this validator's signature for the given attestation data
// is already held. Callers use it to skip re-verifying a duplicate: an XMSS
// verification costs hundreds of milliseconds and the answer cannot change.
func (m *AttestationSignatureMap) Has(dataRoot [32]byte, validatorID uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.data[dataRoot]
	if !ok {
		return false
	}
	for _, sig := range entry.Signatures {
		if sig.ValidatorID == validatorID {
			return true
		}
	}
	return false
}

func (m *AttestationSignatureMap) Delete(keys []AttestationDeleteKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		entry, ok := m.data[key.DataRoot]
		if !ok {
			continue
		}
		removed := 0
		filtered := entry.Signatures[:0]
		for _, sig := range entry.Signatures {
			if sig.ValidatorID != key.ValidatorID {
				filtered = append(filtered, sig)
				continue
			}
			removed++
		}
		m.total -= removed
		entry.Signatures = filtered
		if len(entry.Signatures) == 0 {
			m.dropRootLocked(key.DataRoot)
		}
	}
}

// PruneBelow drops signatures whose target checkpoint is finalized. The target
// is the right key: it is what decides whether a vote can still advance
// finality, and it is what the payload buffers prune on, so the two pools now
// clear the same data roots instead of each keeping what the other dropped.
func (m *AttestationSignatureMap) PruneBelow(finalizedSlot uint64) int {
	return m.pruneWhere(func(entry *AttestationDataEntry) bool {
		return entryTargetSlot(entry) <= finalizedSlot
	})
}

// entryTargetSlot is the slot a vote's staleness is judged on. The target
// checkpoint decides whether the vote can still advance finality; validation
// guarantees one, and a malformed entry without one falls back to the
// attestation slot, matching how orderedGroups reads the same data.
func entryTargetSlot(entry *AttestationDataEntry) uint64 {
	if entry.Data.Target != nil {
		return entry.Data.Target.Slot
	}
	return entry.Data.Slot
}

// PruneStaleBelow drops signatures whose target sits below cutoff. It exists for
// the case PruneBelow cannot cover: while finalization is stalled the finalized
// slot does not move, so a finalization-keyed prune never fires and the pool
// grows for as long as the stall lasts.
//
// ream and grandine exempt roots that carry an aggregated payload from their
// equivalent sweeps, to keep the coverage a live aggregate was built from. That
// exemption cannot apply here: a root's signature entry and its payload entry
// hold the same AttestationData, so they share a target slot and go stale in the
// same sweep. Nothing would ever be exempt.
func (m *AttestationSignatureMap) PruneStaleBelow(cutoff uint64) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pruneLocked(func(_ [32]byte, entry *AttestationDataEntry) bool {
		return entry == nil || entry.Data == nil || entryTargetSlot(entry) < cutoff
	})
}

func (m *AttestationSignatureMap) pruneWhere(stale func(*AttestationDataEntry) bool) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pruneLocked(func(_ [32]byte, entry *AttestationDataEntry) bool {
		return entry == nil || entry.Data == nil || stale(entry)
	})
}

// pruneLocked drops every root the predicate calls stale, then rebuilds the
// insertion order in a single pass. Removing each root from order individually
// would rescan it per root, which is quadratic exactly when a sweep has the most
// to drop.
func (m *AttestationSignatureMap) pruneLocked(stale func([32]byte, *AttestationDataEntry) bool) int {
	pruned := 0
	for root, entry := range m.data {
		if !stale(root, entry) {
			continue
		}
		if entry != nil {
			m.total -= len(entry.Signatures)
		}
		delete(m.data, root)
		pruned++
	}
	if pruned > 0 {
		kept := m.order[:0]
		for _, root := range m.order {
			if _, ok := m.data[root]; ok {
				kept = append(kept, root)
			}
		}
		m.order = kept
	}
	return pruned
}

func (m *AttestationSignatureMap) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.data)
}

// SignatureCountForSlot is the number of collected votes whose attestation data
// is for the given slot. Early aggregation gauges coverage of the slot being
// proved, not the cross-slot backlog still awaiting pruning.
func (m *AttestationSignatureMap) SignatureCountForSlot(slot uint64) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, entry := range m.data {
		if entry.Data != nil && entry.Data.Slot == slot {
			n += len(entry.Signatures)
		}
	}
	return n
}

func (m *AttestationSignatureMap) Snapshot() map[[32]byte]*AttestationDataEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	snap := make(map[[32]byte]*AttestationDataEntry, len(m.data))
	for k, v := range m.data {
		if v == nil || v.Data == nil {
			continue
		}
		signatures := make([]AttestationSignatureEntry, len(v.Signatures))
		copy(signatures, v.Signatures)
		snap[k] = &AttestationDataEntry{Data: copyAttestationData(v.Data), Signatures: signatures}
	}
	return snap
}

type AttestationDeleteKey struct {
	ValidatorID uint64
	DataRoot    [32]byte
}
