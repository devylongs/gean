package store_test

import (
	"testing"

	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

func TestAttestationSignatureInsertAndDelete(t *testing.T) {
	gsm := store.NewAttestationSignatureMap()
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
	gsm := store.NewAttestationSignatureMap()
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

func TestAttestationSignaturePruneBacklog(t *testing.T) {
	gsm := store.NewAttestationSignatureMap()
	var sig [types.SignatureSize]byte

	// Below the window relative to head, nothing is pruned (normal operation:
	// finality lag is far inside the window).
	head := uint64(100)
	gsm.Insert([32]byte{1}, &types.AttestationData{Slot: head - 5}, 0, sig)
	gsm.Insert([32]byte{2}, &types.AttestationData{Slot: head}, 1, sig)
	if pruned := gsm.PruneBacklog(head); pruned != 0 {
		t.Fatalf("in-window prune=%d, want 0", pruned)
	}
	if gsm.Len() != 2 {
		t.Fatalf("in-window Len=%d, want 2", gsm.Len())
	}

	// With head far ahead, roots older than head-window are dropped, recent kept.
	head = store.MaxAttestationBacklogSlots + 1000
	floor := head - store.MaxAttestationBacklogSlots
	gsm.Insert([32]byte{3}, &types.AttestationData{Slot: floor - 1}, 2, sig) // stale, drop
	gsm.Insert([32]byte{4}, &types.AttestationData{Slot: floor + 1}, 3, sig) // recent, keep
	pruned := gsm.PruneBacklog(head)
	// The two slot-100-era roots and the stale root are all below the new floor.
	if pruned != 3 {
		t.Fatalf("backlog prune=%d, want 3", pruned)
	}
	snap := gsm.Snapshot()
	if _, ok := snap[[32]byte{4}]; !ok {
		t.Fatal("recent root was pruned")
	}
	if gsm.Len() != 1 {
		t.Fatalf("post-prune Len=%d, want 1", gsm.Len())
	}

	// Head below the window never underflows / prunes.
	if pruned := gsm.PruneBacklog(store.MaxAttestationBacklogSlots - 1); pruned != 0 {
		t.Fatalf("head-below-window prune=%d, want 0", pruned)
	}
}

func TestAttestationSignaturePruneBelow(t *testing.T) {
	gsm := store.NewAttestationSignatureMap()
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
	gsm := store.NewAttestationSignatureMap()
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
	gsm := store.NewAttestationSignatureMap()
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
