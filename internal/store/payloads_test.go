package store_test

import (
	"sync"
	"testing"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestPayloadBufferPushAndExtract(t *testing.T) {
	pb := store.NewPayloadBuffer(100)
	var dr [32]byte
	dr[0] = 1
	data := &types.AttestationData{Slot: 5}
	participants := types.NewBitlistSSZ(3)
	types.BitlistSet(participants, 0)
	types.BitlistSet(participants, 2)
	proof := &types.SingleMessageAggregate{Participants: participants, Proof: []byte{0x01}}

	pb.Push(dr, data, proof)
	if pb.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", pb.Len())
	}

	atts := pb.ExtractLatestAttestations()
	if len(atts) != 2 {
		t.Fatalf("expected 2 validators, got %d", len(atts))
	}
	if atts[0].Slot != 5 || atts[2].Slot != 5 {
		t.Fatal("attestation data mismatch")
	}
}

// An equivocator can cast two distinct votes at one slot. Resolving that tie by the
// larger canonical data root (leanSpec #1181) must make the extracted LMD view a pure
// function of the pool, independent of insertion order, so every node runs fork choice
// on the same set.
func TestPayloadBufferEqualSlotTieKeepsLargerDataRoot(t *testing.T) {
	small := [32]byte{0x01}
	large := [32]byte{0x02}
	smallData := &types.AttestationData{Slot: 7, Head: &types.Checkpoint{Root: small, Slot: 6}}
	largeData := &types.AttestationData{Slot: 7, Head: &types.Checkpoint{Root: large, Slot: 6}}
	bits := types.BitlistFromIndices([]uint64{0})

	for _, largeFirst := range []bool{false, true} {
		pb := store.NewPayloadBuffer(10)
		push := func(root [32]byte, d *types.AttestationData) {
			pb.Push(root, d, &types.SingleMessageAggregate{Participants: bits, Proof: []byte{0x01}})
		}
		if largeFirst {
			push(large, largeData)
			push(small, smallData)
		} else {
			push(small, smallData)
			push(large, largeData)
		}

		got := pb.ExtractLatestAttestations()[0]
		if got == nil {
			t.Fatalf("largeFirst=%v: validator 0 has no extracted vote", largeFirst)
		}
		if got.Head.Root != large {
			t.Fatalf("largeFirst=%v: kept vote with head 0x%x, want larger-root vote 0x%x",
				largeFirst, got.Head.Root[:2], large[:2])
		}
	}
}

func TestPayloadBufferDataOnlyHasNoWeight(t *testing.T) {
	pb := store.NewPayloadBuffer(10)
	root := [32]byte{1}
	pb.PushData(root, &types.AttestationData{Slot: 3})

	if pb.Len() != 1 || pb.TotalProofs() != 0 {
		t.Fatalf("entries=%d proofs=%d, want 1,0", pb.Len(), pb.TotalProofs())
	}
	if len(pb.Entries()) != 0 || len(pb.ExtractLatestAttestations()) != 0 {
		t.Fatal("data-only payload became selectable or gained head weight")
	}
}

func TestPayloadBufferPushPrunesSubsetProofs(t *testing.T) {
	pb := store.NewPayloadBuffer(10)
	root := [32]byte{1}
	data := &types.AttestationData{Slot: 3}
	for _, index := range []uint64{0, 1} {
		pb.Push(root, data, &types.SingleMessageAggregate{
			Participants: types.BitlistFromIndices([]uint64{index}),
			Proof:        []byte{byte(index + 1)},
		})
	}
	combined := &types.SingleMessageAggregate{
		Participants: types.BitlistFromIndices([]uint64{0, 1}),
		Proof:        []byte{3},
	}
	pb.Push(root, data, combined)

	entry := pb.Entries()[root]
	if pb.TotalProofs() != 1 || entry == nil || len(entry.Proofs) != 1 {
		t.Fatalf("proofs=%d entry=%v", pb.TotalProofs(), entry)
	}
}

func TestPayloadBufferPushPreservesOverlappingProofs(t *testing.T) {
	pb := store.NewPayloadBuffer(10)
	root := [32]byte{1}
	data := &types.AttestationData{Slot: 3}
	for i, indices := range [][]uint64{{0, 1}, {1, 2}} {
		pb.Push(root, data, &types.SingleMessageAggregate{
			Participants: types.BitlistFromIndices(indices),
			Proof:        []byte{byte(i + 1)},
		})
	}

	if got := pb.TotalProofs(); got != 2 {
		t.Fatalf("proofs=%d, want 2", got)
	}
}

func TestPayloadBufferFIFOEviction(t *testing.T) {
	pb := store.NewPayloadBuffer(2)

	for i := range byte(5) {
		var dr [32]byte
		dr[0] = i
		data := &types.AttestationData{Slot: uint64(i)}
		bits := types.NewBitlistSSZ(1)
		types.BitlistSet(bits, 0)
		proof := &types.SingleMessageAggregate{Participants: bits, Proof: []byte{0x01}}
		pb.Push(dr, data, proof)
	}

	if pb.TotalProofs() > 2 {
		t.Fatalf("expected <= 2 proofs, got %d", pb.TotalProofs())
	}
}

func TestPayloadBufferRejectsMalformedPush(t *testing.T) {
	pb := store.NewPayloadBuffer(10)
	var dr [32]byte

	pb.Push(dr, nil, &types.SingleMessageAggregate{Participants: types.NewBitlistSSZ(1)})
	pb.Push(dr, &types.AttestationData{}, nil)
	pb.Push(dr, &types.AttestationData{}, &types.SingleMessageAggregate{})
	pb.Push(dr, &types.AttestationData{}, &types.SingleMessageAggregate{
		Participants: types.NewBitlistSSZ(1),
		Proof:        []byte{0x01},
	})

	if pb.Len() != 0 || pb.TotalProofs() != 0 {
		t.Fatalf("malformed pushes stored entries=%d proofs=%d, want 0,0", pb.Len(), pb.TotalProofs())
	}
}

func TestPayloadBufferEntriesReturnsSnapshot(t *testing.T) {
	pb := store.NewPayloadBuffer(10)
	var dr [32]byte
	dr[0] = 1
	bits := types.NewBitlistSSZ(1)
	types.BitlistSet(bits, 0)
	proof := &types.SingleMessageAggregate{Participants: bits, Proof: []byte{0x01}}
	data := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Slot: 1, Root: [32]byte{0x01}},
		Target: &types.Checkpoint{Slot: 1, Root: [32]byte{0x02}},
		Source: &types.Checkpoint{Slot: 0, Root: [32]byte{0x03}},
	}

	pb.Push(dr, data, proof)
	data.Head.Root = [32]byte{0xff}
	proof.Participants[0] = 0xff
	proof.Proof[0] = 0xff

	entries := pb.Entries()
	if entries[dr].Data.Head.Root[0] != 0x01 {
		t.Fatal("Push leaked mutable attestation data into payload buffer")
	}
	if entries[dr].Proofs[0].Participants[0] == 0xff || entries[dr].Proofs[0].Proof[0] == 0xff {
		t.Fatal("Push leaked mutable proof data into payload buffer")
	}

	entries[dr].Proofs[0] = nil
	entries[dr].Data.Head.Root = [32]byte{0xee}
	delete(entries, dr)

	fresh := pb.Entries()
	if fresh[dr] == nil || fresh[dr].Proofs[0] == nil {
		t.Fatal("Entries leaked mutable internal map or proof slice")
	}
	if fresh[dr].Data.Head.Root[0] != 0x01 {
		t.Fatal("Entries leaked mutable attestation data")
	}
}

func TestPayloadBufferConcurrentAccess(t *testing.T) {
	pb := store.NewPayloadBuffer(0)
	var wg sync.WaitGroup

	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				var dr [32]byte
				dr[0] = byte(worker)
				dr[1] = byte(i)
				bits := types.NewBitlistSSZ(4)
				types.BitlistSet(bits, uint64(i%4))
				pb.Push(dr, &types.AttestationData{Slot: uint64(i)}, &types.SingleMessageAggregate{
					Participants: bits,
					Proof:        []byte{byte(i)},
				})
				_ = pb.Len()
				_ = pb.TotalProofs()
				_ = pb.Entries()
				_ = pb.ExtractLatestAttestations()
			}
		}(worker)
	}

	wg.Wait()
	if pb.TotalProofs() == 0 {
		t.Fatal("expected concurrent pushes to store proofs")
	}
}

func TestPromoteNewToKnown(t *testing.T) {
	s := makeTestStore()

	var dr [32]byte
	dr[0] = 1
	data := &types.AttestationData{Slot: 5}
	bits := types.NewBitlistSSZ(1)
	types.BitlistSet(bits, 0)
	proof := &types.SingleMessageAggregate{Participants: bits, Proof: []byte{0x01}}

	s.NewPayloads.Push(dr, data, proof)
	if s.NewPayloads.Len() != 1 {
		t.Fatal("expected 1 new payload")
	}
	if s.KnownPayloads.Len() != 0 {
		t.Fatal("known should be empty")
	}

	s.PromoteNewToKnown()

	if s.NewPayloads.Len() != 0 {
		t.Fatal("new should be empty after promote")
	}
	if s.KnownPayloads.Len() != 1 {
		t.Fatal("known should have 1 entry")
	}
}

func TestExtractLatestNewAttestations(t *testing.T) {
	s := makeTestStore()

	var dr1 [32]byte
	dr1[0] = 1
	bits1 := types.NewBitlistSSZ(2)
	types.BitlistSet(bits1, 0)
	s.KnownPayloads.Push(dr1, &types.AttestationData{Slot: 5}, &types.SingleMessageAggregate{Participants: bits1, Proof: []byte{0x01}})

	var dr2 [32]byte
	dr2[0] = 2
	bits2 := types.NewBitlistSSZ(2)
	types.BitlistSet(bits2, 1)
	s.NewPayloads.Push(dr2, &types.AttestationData{Slot: 8}, &types.SingleMessageAggregate{Participants: bits2, Proof: []byte{0x01}})

	got := s.ExtractLatestNewAttestations()
	if _, found := got[0]; found {
		t.Fatal("validator 0 lives only in known pool; must not appear")
	}
	if got[1] == nil || got[1].Slot != 8 {
		t.Fatalf("validator 1 should appear at slot 8, got %v", got[1])
	}
}
