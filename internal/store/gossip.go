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

func (m *AttestationSignatureMap) PruneBelow(finalizedSlot uint64) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	pruned := 0
	for root, entry := range m.data {
		if entry == nil || entry.Data == nil || entry.Data.Slot <= finalizedSlot {
			if entry != nil {
				m.total -= len(entry.Signatures)
			}
			m.dropRootLocked(root)
			pruned++
		}
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
