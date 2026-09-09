package storage

import (
	"bytes"
	"os"
	"testing"
)

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gean-pebble-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestPebblePutAndGet(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableBlockHeaders, []KV{
		{Key: []byte("root1"), Value: []byte("header1")},
		{Key: []byte("root2"), Value: []byte("header2")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, _ := rv.Get(TableBlockHeaders, []byte("root1"))
	if string(val) != "header1" {
		t.Fatalf("expected header1, got %s", string(val))
	}
	val, _ = rv.Get(TableBlockHeaders, []byte("root2"))
	if string(val) != "header2" {
		t.Fatalf("expected header2, got %s", string(val))
	}
}

func TestPebbleGetMissing(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	rv, _ := b.BeginRead()
	val, _ := rv.Get(TableBlockHeaders, []byte("nonexistent"))
	if val != nil {
		t.Fatal("expected nil for missing key")
	}
}

func TestPebblePutNilValueStoresEmptyValue(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableMetadata, []KV{{Key: []byte("key"), Value: nil}}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, err := rv.Get(TableMetadata, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if val == nil || len(val) != 0 {
		t.Fatalf("value=%v, want non-nil empty value", val)
	}
}

func TestPebbleDelete(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableStates, []KV{
		{Key: []byte("k1"), Value: []byte("v1")},
		{Key: []byte("k2"), Value: []byte("v2")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	wb2, _ := b.BeginWrite()
	if err := wb2.DeleteBatch(TableStates, [][]byte{[]byte("k1")}); err != nil {
		t.Fatal(err)
	}
	if err := wb2.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, _ := rv.Get(TableStates, []byte("k1"))
	if val != nil {
		t.Fatal("k1 should be deleted")
	}
	val, _ = rv.Get(TableStates, []byte("k2"))
	if string(val) != "v2" {
		t.Fatal("k2 should still exist")
	}
}

func TestPebbleTableIsolation(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableBlockHeaders, []KV{
		{Key: []byte("root"), Value: []byte("header")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, _ := rv.Get(TableStates, []byte("root"))
	if val != nil {
		t.Fatal("tables should be isolated")
	}
}

func TestPebblePrefixIterator(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableLiveChain, []KV{
		{Key: []byte("aa_1"), Value: []byte("v1")},
		{Key: []byte("aa_2"), Value: []byte("v2")},
		{Key: []byte("bb_1"), Value: []byte("v3")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	it, err := rv.PrefixIterator(TableLiveChain, []byte("aa"))
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	count := 0
	for it.Next() {
		if !bytes.HasPrefix(it.Key(), []byte("aa")) {
			t.Fatalf("key %s doesn't have prefix aa", string(it.Key()))
		}
		count++
	}
	if count != 2 {
		t.Fatalf("expected 2 entries with prefix aa, got %d", count)
	}
}

func TestPebbleReadResultsAreCallerOwned(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableMetadata, []KV{{Key: []byte("key"), Value: []byte("value")}}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	val, err := rv.Get(TableMetadata, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	val[0] = 'X'

	fresh, err := rv.Get(TableMetadata, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if string(fresh) != "value" {
		t.Fatalf("stored value mutated through Get result: %q", string(fresh))
	}

	it, err := rv.PrefixIterator(TableMetadata, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	if !it.Next() {
		t.Fatal("expected iterator entry")
	}
	key := it.Key()
	iterVal := it.Value()
	key[0] = 'X'
	iterVal[0] = 'X'
	if string(it.Key()) != "key" || string(it.Value()) != "value" {
		t.Fatal("iterator key/value should be caller-owned copies")
	}
}

func TestPebblePrefixIteratorSupportsEmptyKey(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableMetadata, []KV{{Key: nil, Value: []byte("empty")}}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	rv, _ := b.BeginRead()
	it, err := rv.PrefixIterator(TableMetadata, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	if !it.Next() {
		t.Fatal("expected empty-key entry")
	}
	if key := it.Key(); key == nil || len(key) != 0 {
		t.Fatalf("key=%v, want non-nil empty slice", key)
	}
	if string(it.Value()) != "empty" {
		t.Fatalf("value=%q, want empty", string(it.Value()))
	}
}

func TestPebblePersistence(t *testing.T) {
	dir := tempDir(t)

	{
		b, err := NewPebbleBackend(dir)
		if err != nil {
			t.Fatal(err)
		}
		wb, _ := b.BeginWrite()
		if err := wb.PutBatch(TableMetadata, []KV{
			{Key: []byte("key"), Value: []byte("value")},
		}); err != nil {
			t.Fatal(err)
		}
		if err := wb.Commit(); err != nil {
			t.Fatal(err)
		}
		if err := b.Close(); err != nil {
			t.Fatal(err)
		}
	}

	{
		b, err := NewPebbleBackend(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer b.Close()
		rv, _ := b.BeginRead()
		val, _ := rv.Get(TableMetadata, []byte("key"))
		if string(val) != "value" {
			t.Fatalf("expected value after reopen, got %s", string(val))
		}
	}
}

func TestPebbleEstimateTableBytes(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableMetadata, []KV{
		{Key: []byte("k"), Value: []byte("value")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.PutBatch(TableStates, []KV{
		{Key: []byte("k"), Value: []byte("larger-value")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}

	// EstimateTableBytes reports SST bytes for the table's key range, so it sees
	// nothing until the memtable is flushed. That is the documented contract:
	// the gauge is coarse and sampled on a slow cadence, and the alternative —
	// summing every key and value — is a full-table scan.
	if size := b.EstimateTableBytes(TableMetadata); size != 0 {
		t.Fatalf("unflushed metadata size=%d, want 0", size)
	}

	if err := b.db.Flush(); err != nil {
		t.Fatal(err)
	}

	metaSize := b.EstimateTableBytes(TableMetadata)
	if metaSize == 0 {
		t.Fatal("metadata size=0 after flush, want non-zero")
	}

	// Each table is estimated over its own prefix range, so a write to one must
	// not be attributed to another. TableBlockHeaders was never written.
	if size := b.EstimateTableBytes(TableBlockHeaders); size != 0 {
		t.Fatalf("unwritten table size=%d, want 0", size)
	}
	if statesSize := b.EstimateTableBytes(TableStates); statesSize == 0 {
		t.Fatal("states size=0 after flush, want non-zero")
	}
}

func TestPebbleWriteBatchClosedAfterCommit(t *testing.T) {
	b, err := NewPebbleBackend(tempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	wb, _ := b.BeginWrite()
	if err := wb.PutBatch(TableMetadata, []KV{{Key: []byte("key"), Value: []byte("value")}}); err != nil {
		t.Fatal(err)
	}
	if err := wb.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := wb.PutBatch(TableMetadata, []KV{{Key: []byte("next"), Value: []byte("value")}}); err != errBatchClosed {
		t.Fatalf("PutBatch after commit error=%v, want %v", err, errBatchClosed)
	}
	if err := wb.DeleteBatch(TableMetadata, [][]byte{[]byte("key")}); err != errBatchClosed {
		t.Fatalf("DeleteBatch after commit error=%v, want %v", err, errBatchClosed)
	}
	if err := wb.Commit(); err != errBatchClosed {
		t.Fatalf("Commit after commit error=%v, want %v", err, errBatchClosed)
	}
}
