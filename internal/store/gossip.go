package store

import (
	"sync"
	"unsafe"

	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

type AttestationSignatureEntry struct {
	ValidatorID uint64
	Signature   [types.SignatureSize]byte
	SigHandle   unsafe.Pointer
}

type AttestationDataEntry struct {
	Data       *types.AttestationData
	Signatures []AttestationSignatureEntry
}

type AttestationSignatureMap struct {
	mu   sync.Mutex
	data map[[32]byte]*AttestationDataEntry
}

func NewAttestationSignatureMap() AttestationSignatureMap {
	return AttestationSignatureMap{data: make(map[[32]byte]*AttestationDataEntry)}
}

func (m *AttestationSignatureMap) Insert(dataRoot [32]byte, data *types.AttestationData, validatorID uint64, sig [types.SignatureSize]byte) {
	m.InsertWithHandle(dataRoot, data, validatorID, sig, nil, nil)
}

func (m *AttestationSignatureMap) InsertWithHandle(dataRoot [32]byte, data *types.AttestationData, validatorID uint64, sig [types.SignatureSize]byte, handle unsafe.Pointer, parseErr error) {
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
	}
	var h unsafe.Pointer
	if parseErr == nil {
		h = handle
	}
	entry.Signatures = append(entry.Signatures, AttestationSignatureEntry{
		ValidatorID: validatorID,
		Signature:   sig,
		SigHandle:   h,
	})
}

func (m *AttestationSignatureMap) Delete(keys []AttestationDeleteKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		entry, ok := m.data[key.DataRoot]
		if !ok {
			continue
		}
		filtered := entry.Signatures[:0]
		for _, sig := range entry.Signatures {
			if sig.ValidatorID == key.ValidatorID {
				if sig.SigHandle != nil {
					xmss.FreeSignature(sig.SigHandle)
				}
			} else {
				filtered = append(filtered, sig)
			}
		}
		entry.Signatures = filtered
		if len(entry.Signatures) == 0 {
			delete(m.data, key.DataRoot)
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
				for _, sig := range entry.Signatures {
					if sig.SigHandle != nil {
						xmss.FreeSignature(sig.SigHandle)
					}
				}
			}
			delete(m.data, root)
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

// MaxAttestationBacklogSlots bounds how many slots of pending attestation
// signatures are kept behind the head. Signatures are otherwise pruned only
// below finalized, so a stalled finalization lets the backlog grow without
// bound — inflating each aggregation session and feeding the stall. Beyond this
// window (relative to head) the votes are too old to justify a still-unfinalized
// checkpoint, so dropping them bounds memory and session size while leaving
// normal operation untouched: finality lag is single- to low-double-digit slots,
// far inside the window.
const MaxAttestationBacklogSlots = 512

// PruneBacklog drops pending signatures for slots more than
// MaxAttestationBacklogSlots behind headSlot, bounding the backlog when
// finalization has stalled. Returns the number of data roots removed.
func (m *AttestationSignatureMap) PruneBacklog(headSlot uint64) int {
	if headSlot <= MaxAttestationBacklogSlots {
		return 0
	}
	return m.PruneBelow(headSlot - MaxAttestationBacklogSlots)
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
