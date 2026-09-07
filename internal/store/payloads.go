package store

import (
	"bytes"
	"sync"

	"github.com/geanlabs/gean/internal/types"
)

type PayloadEntry struct {
	Data   *types.AttestationData
	Proofs []*types.SingleMessageAggregate
}

type PayloadBuffer struct {
	mu          sync.Mutex
	data        map[[32]byte]*PayloadEntry
	order       [][32]byte
	capacity    int
	totalProofs int
}

func NewPayloadBuffer(capacity int) *PayloadBuffer {
	return &PayloadBuffer{
		data:     make(map[[32]byte]*PayloadEntry),
		capacity: capacity,
	}
}

func (pb *PayloadBuffer) PushData(dataRoot [32]byte, attData *types.AttestationData) {
	if pb == nil || attData == nil {
		return
	}
	pb.mu.Lock()
	defer pb.mu.Unlock()
	if _, ok := pb.data[dataRoot]; ok {
		return
	}
	pb.data[dataRoot] = &PayloadEntry{Data: copyAttestationData(attData)}
	pb.order = append(pb.order, dataRoot)
}

func (pb *PayloadBuffer) Push(dataRoot [32]byte, attData *types.AttestationData, proof *types.SingleMessageAggregate) {
	if pb == nil {
		return
	}
	if attData == nil || !validPayloadProof(proof) {
		return
	}

	storedData := copyAttestationData(attData)
	storedProof := copyProof(proof)

	pb.mu.Lock()
	defer pb.mu.Unlock()
	if pb.data == nil {
		pb.data = make(map[[32]byte]*PayloadEntry)
	}
	if entry, ok := pb.data[dataRoot]; ok {
		for _, existing := range entry.Proofs {
			if validPayloadProof(existing) && bitlistContains(existing.Participants, proof.Participants) {
				return
			}
		}
		kept := entry.Proofs[:0]
		for _, existing := range entry.Proofs {
			if bitlistContains(proof.Participants, existing.Participants) {
				pb.totalProofs--
				continue
			}
			kept = append(kept, existing)
		}
		entry.Proofs = append(kept, storedProof)
		pb.totalProofs++
	} else {
		pb.data[dataRoot] = &PayloadEntry{
			Data:   storedData,
			Proofs: []*types.SingleMessageAggregate{storedProof},
		}
		pb.order = append(pb.order, dataRoot)
		pb.totalProofs++
	}

	if pb.capacity > 0 {
		for pb.totalProofs > pb.capacity && len(pb.order) > 0 {
			evicted := pb.order[0]
			pb.order = pb.order[1:]
			if entry, ok := pb.data[evicted]; ok {
				pb.totalProofs -= len(entry.Proofs)
				delete(pb.data, evicted)
			}
		}
	}
}

func (pb *PayloadBuffer) PushBatch(entries []PayloadKV) {
	if pb == nil {
		return
	}
	for _, e := range entries {
		pb.Push(e.DataRoot, e.Data, e.Proof)
	}
}

func (pb *PayloadBuffer) Drain() []PayloadKV {
	if pb == nil {
		return nil
	}
	pb.mu.Lock()
	defer pb.mu.Unlock()

	result := make([]PayloadKV, 0, pb.totalProofs)
	for _, dataRoot := range pb.order {
		entry := pb.data[dataRoot]
		if !validPayloadEntry(entry) {
			continue
		}
		for _, proof := range entry.Proofs {
			if validPayloadProof(proof) {
				result = append(result, PayloadKV{
					DataRoot: dataRoot,
					Data:     copyAttestationData(entry.Data),
					Proof:    copyProof(proof),
				})
			}
		}
	}
	pb.data = make(map[[32]byte]*PayloadEntry)
	pb.order = nil
	pb.totalProofs = 0
	return result
}

func (pb *PayloadBuffer) Len() int {
	if pb == nil {
		return 0
	}
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return len(pb.data)
}

func (pb *PayloadBuffer) TotalProofs() int {
	if pb == nil {
		return 0
	}
	pb.mu.Lock()
	defer pb.mu.Unlock()
	return pb.totalProofs
}

func (pb *PayloadBuffer) ExtractLatestAttestations() map[uint64]*types.AttestationData {
	result := make(map[uint64]*types.AttestationData)
	if pb == nil {
		return result
	}
	pb.mu.Lock()
	defer pb.mu.Unlock()

	// Each validator keeps the vote with the highest (slot, canonical data root).
	// An equivocator can cast two distinct votes at one slot; resolving that tie by
	// the larger data root makes the extracted set a pure function of the pool,
	// independent of insertion order, so every node runs fork choice on the same LMD
	// view. dataRoot is the attestation-data hash-tree-root (the buffer key).
	chosenRoot := make(map[uint64][32]byte)
	for _, dataRoot := range pb.order {
		entry, ok := pb.data[dataRoot]
		if !ok || !validPayloadEntry(entry) {
			continue
		}
		for _, proof := range entry.Proofs {
			if !validPayloadProof(proof) {
				continue
			}
			participantLen := types.BitlistLen(proof.Participants)
			for vid := range participantLen {
				if !types.BitlistGet(proof.Participants, vid) {
					continue
				}
				if existing, ok := result[vid]; ok &&
					!outranksVote(entry.Data.Slot, dataRoot, existing.Slot, chosenRoot[vid]) {
					continue
				}
				result[vid] = copyAttestationData(entry.Data)
				chosenRoot[vid] = dataRoot
			}
		}
	}
	return result
}

// outranksVote reports whether (slotA, rootA) is the later LMD vote than
// (slotB, rootB): a higher slot wins, and an equal-slot tie breaks toward the
// larger canonical data root.
func outranksVote(slotA uint64, rootA [32]byte, slotB uint64, rootB [32]byte) bool {
	if slotA != slotB {
		return slotA > slotB
	}
	return bytes.Compare(rootA[:], rootB[:]) > 0
}

// Roots reports the data roots the buffer currently holds. Callers use it to
// protect a root's raw signatures while an aggregate built from them is live.
func (pb *PayloadBuffer) Roots(into map[[32]byte]bool) {
	if pb == nil || into == nil {
		return
	}
	pb.mu.Lock()
	defer pb.mu.Unlock()
	for root := range pb.data {
		into[root] = true
	}
}

func (pb *PayloadBuffer) PruneBelow(finalizedSlot uint64) int {
	return pb.pruneBelow(finalizedSlot, false)
}

// PruneStaleBelow drops entries whose target sits strictly below cutoff. Unlike
// PruneBelow it is not keyed on finalization, so it still clears the buffer
// while finalization is stalled — the one situation where nothing else does.
func (pb *PayloadBuffer) PruneStaleBelow(cutoff uint64) int {
	return pb.pruneBelow(cutoff, true)
}

func (pb *PayloadBuffer) pruneBelow(bound uint64, strict bool) int {
	if pb == nil {
		return 0
	}
	pb.mu.Lock()
	defer pb.mu.Unlock()

	stale := func(slot uint64) bool {
		if strict {
			return slot < bound
		}
		return slot <= bound
	}

	pruned := 0
	var newOrder [][32]byte
	for _, dataRoot := range pb.order {
		entry, ok := pb.data[dataRoot]
		if !ok {
			continue
		}
		if !validPayloadEntry(entry) || entry.Data.Target == nil || stale(entry.Data.Target.Slot) {
			pb.totalProofs -= len(entry.Proofs)
			delete(pb.data, dataRoot)
			pruned++
		} else {
			newOrder = append(newOrder, dataRoot)
		}
	}
	pb.order = newOrder
	return pruned
}

func (pb *PayloadBuffer) Entries() map[[32]byte]*PayloadEntry {
	if pb == nil {
		return map[[32]byte]*PayloadEntry{}
	}
	pb.mu.Lock()
	defer pb.mu.Unlock()

	out := make(map[[32]byte]*PayloadEntry, len(pb.data))
	for _, dataRoot := range pb.order {
		entry, ok := pb.data[dataRoot]
		if !ok || !validPayloadEntry(entry) {
			continue
		}
		proofs := make([]*types.SingleMessageAggregate, 0, len(entry.Proofs))
		for _, proof := range entry.Proofs {
			if validPayloadProof(proof) {
				proofs = append(proofs, copyProof(proof))
			}
		}
		if len(proofs) == 0 {
			continue
		}
		out[dataRoot] = &PayloadEntry{Data: copyAttestationData(entry.Data), Proofs: proofs}
	}
	return out
}

type PayloadKV struct {
	DataRoot [32]byte
	Data     *types.AttestationData
	Proof    *types.SingleMessageAggregate
}

func (s *ConsensusStore) PromoteNewToKnown() {
	if s == nil || s.NewPayloads == nil || s.KnownPayloads == nil {
		return
	}
	s.KnownPayloads.PushBatch(s.NewPayloads.Drain())
}

func (s *ConsensusStore) ExtractLatestKnownAttestations() map[uint64]*types.AttestationData {
	if s == nil || s.KnownPayloads == nil {
		return map[uint64]*types.AttestationData{}
	}
	return s.KnownPayloads.ExtractLatestAttestations()
}

func (s *ConsensusStore) ExtractLatestNewAttestations() map[uint64]*types.AttestationData {
	if s == nil || s.NewPayloads == nil {
		return map[uint64]*types.AttestationData{}
	}
	return s.NewPayloads.ExtractLatestAttestations()
}

func validPayloadEntry(entry *PayloadEntry) bool {
	return entry != nil && entry.Data != nil
}

func validPayloadProof(proof *types.SingleMessageAggregate) bool {
	return proof != nil &&
		len(proof.Proof) > 0 &&
		types.BitlistCount(proof.Participants) > 0
}

func bitlistContains(superset, subset []byte) bool {
	for _, index := range types.BitlistIndices(subset) {
		if !types.BitlistGet(superset, index) {
			return false
		}
	}
	return true
}
