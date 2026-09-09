package store_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestAttestationSignatureInsertAndDelete(t *testing.T) {
	gsm := store.NewAttestationSignatureMap(0)
	var dr [32]byte
	dr[0] = 1
	data := &types.AttestationData{Slot: 5}
	var sig [types.SignatureSize]byte

	gsm.Insert(dr, data, 0, sig)
	gsm.Insert(dr, data, 1, sig)

	if gsm.Len() != 1 {
		t.Fatalf("expected 1 entry, got %d", gsm.Len())
	}
	snap := gsm.Snapshot()
	if len(snap[dr].Signatures) != 2 {
		t.Fatal("expected 2 signatures")
	}

	gsm.Delete([]store.AttestationDeleteKey{{ValidatorID: 0, DataRoot: dr}})
	snap = gsm.Snapshot()
	if len(snap[dr].Signatures) != 1 {
		t.Fatal("expected 1 signature after delete")
	}
}

func TestAttestationSignatureCountForSlot(t *testing.T) {
	gsm := store.NewAttestationSignatureMap(0)
	var sig [types.SignatureSize]byte

	if gsm.SignatureCountForSlot(1) != 0 {
		t.Fatalf("empty map count=%d, want 0", gsm.SignatureCountForSlot(1))
	}

	// Slot 1: three votes on one root, one on another → 4. Slot 2: one vote.
	// The count must be scoped to the queried slot, never the cross-slot total.
	dr1, dr2, dr3 := [32]byte{1}, [32]byte{2}, [32]byte{3}
	gsm.Insert(dr1, &types.AttestationData{Slot: 1}, 0, sig)
	gsm.Insert(dr1, &types.AttestationData{Slot: 1}, 1, sig)
	gsm.Insert(dr1, &types.AttestationData{Slot: 1}, 2, sig)
	gsm.Insert(dr2, &types.AttestationData{Slot: 1}, 3, sig)
	gsm.Insert(dr3, &types.AttestationData{Slot: 2}, 4, sig)

	if got := gsm.SignatureCountForSlot(1); got != 4 {
		t.Fatalf("SignatureCountForSlot(1)=%d, want 4", got)
	}
	if got := gsm.SignatureCountForSlot(2); got != 1 {
		t.Fatalf("SignatureCountForSlot(2)=%d, want 1", got)
	}
	if got := gsm.SignatureCountForSlot(3); got != 0 {
		t.Fatalf("SignatureCountForSlot(3)=%d, want 0", got)
	}
}

func TestAttestationSignaturePruneBelow(t *testing.T) {
	gsm := store.NewAttestationSignatureMap(0)
	var sig [types.SignatureSize]byte
	for i := range uint64(5) {
		var dr [32]byte
		dr[0] = byte(i)
		gsm.Insert(dr, &types.AttestationData{Slot: i}, 0, sig)
	}

	pruned := gsm.PruneBelow(2)
	if pruned != 3 {
		t.Fatalf("expected 3 pruned, got %d", pruned)
	}
	if gsm.Len() != 2 {
		t.Fatalf("expected 2 remaining, got %d", gsm.Len())
	}
}

func TestAttestationSignatureRejectsNilData(t *testing.T) {
	gsm := store.NewAttestationSignatureMap(0)
	var sig [types.SignatureSize]byte

	gsm.Insert([32]byte{1}, nil, 0, sig)

	if gsm.Len() != 0 {
		t.Fatalf("nil data inserted entries=%d, want 0", gsm.Len())
	}
}

func TestAttestationSignatureMapZeroValueInsert(t *testing.T) {
	var gsm store.AttestationSignatureMap
	var sig [types.SignatureSize]byte
	gsm.Insert([32]byte{1}, &types.AttestationData{Slot: 1}, 0, sig)

	if gsm.Len() != 1 {
		t.Fatalf("zero-value map entries=%d, want 1", gsm.Len())
	}
}

func TestAttestationSignatureSnapshotReturnsCopy(t *testing.T) {
	gsm := store.NewAttestationSignatureMap(0)
	var dr [32]byte
	dr[0] = 1
	var sig [types.SignatureSize]byte
	data := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{Slot: 1, Root: [32]byte{0x01}},
		Target: &types.Checkpoint{Slot: 1, Root: [32]byte{0x02}},
		Source: &types.Checkpoint{Slot: 0, Root: [32]byte{0x03}},
	}

	gsm.Insert(dr, data, 0, sig)
	gsm.Insert(dr, data, 1, sig)
	data.Head.Root = [32]byte{0xff}

	snap := gsm.Snapshot()
	snap[dr].Signatures = nil
	snap[dr].Data.Head.Root = [32]byte{0xee}

	fresh := gsm.Snapshot()
	if len(fresh[dr].Signatures) != 2 {
		t.Fatalf("snapshot mutation changed store signatures=%d, want 2", len(fresh[dr].Signatures))
	}
	if fresh[dr].Data.Head.Root[0] != 0x01 {
		t.Fatal("snapshot mutation changed store attestation data")
	}
}

// The pools are pruned only when finalization advances, and a finality stall is
// exactly when that stops. Without a bound the signature map grows for as long
// as the stall lasts, which is the shape observed on devnet-5.
func TestAttestationSignatureMapEvictsOldestRootAtCapacity(t *testing.T) {
	gsm := store.NewAttestationSignatureMap(3)

	data := func(slot uint64) *types.AttestationData {
		return &types.AttestationData{
			Slot:   slot,
			Head:   &types.Checkpoint{},
			Target: &types.Checkpoint{Slot: slot},
			Source: &types.Checkpoint{},
		}
	}
	var root [3][32]byte
	for i := range root {
		root[i][0] = byte(i + 1)
	}

	// Two signatures on the first root, one on the second: at capacity.
	gsm.Insert(root[0], data(1), 0, [types.SignatureSize]byte{})
	gsm.Insert(root[0], data(1), 1, [types.SignatureSize]byte{})
	gsm.Insert(root[1], data(2), 0, [types.SignatureSize]byte{})
	if got := gsm.Len(); got != 2 {
		t.Fatalf("roots=%d, want 2 before eviction", got)
	}

	// The next signature pushes past capacity and takes the oldest root whole,
	// so the roots that survive still carry every vote they collected.
	gsm.Insert(root[2], data(3), 0, [types.SignatureSize]byte{})

	snap := gsm.Snapshot()
	if _, ok := snap[root[0]]; ok {
		t.Fatal("oldest root survived eviction")
	}
	if len(snap[root[1]].Signatures) != 1 || len(snap[root[2]].Signatures) != 1 {
		t.Fatal("eviction damaged a surviving root")
	}
}

// Gossip meshes deliver the same attestation more than once, and the
// pending-attestation replay path re-enters the handler for buffered votes.
// Storing a vote twice wastes memory and, worse, overstates how many distinct
// validators have voted — the count the early-aggregation trigger reads.
func TestAttestationSignatureMapIgnoresDuplicateValidator(t *testing.T) {
	gsm := store.NewAttestationSignatureMap(0)
	data := &types.AttestationData{
		Slot:   1,
		Head:   &types.Checkpoint{},
		Target: &types.Checkpoint{Slot: 1},
		Source: &types.Checkpoint{},
	}
	var root [32]byte
	root[0] = 1

	if gsm.Has(root, 0) {
		t.Fatal("empty map reported a signature it does not hold")
	}

	gsm.Insert(root, data, 0, [types.SignatureSize]byte{})
	gsm.Insert(root, data, 0, [types.SignatureSize]byte{})
	gsm.Insert(root, data, 1, [types.SignatureSize]byte{})

	snap := gsm.Snapshot()
	if got := len(snap[root].Signatures); got != 2 {
		t.Fatalf("signatures=%d, want 2 (one per validator)", got)
	}
	if !gsm.Has(root, 0) || !gsm.Has(root, 1) {
		t.Fatal("stored signature not reported as held")
	}
	if gsm.Has(root, 2) {
		t.Fatal("reported a signature that was never inserted")
	}
}
