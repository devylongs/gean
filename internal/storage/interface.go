package storage

type Backend interface {
	BeginRead() (ReadView, error)

	BeginWrite() (WriteBatch, error)

	EstimateTableBytes(table Table) uint64

	Close() error
}

type ReadView interface {
	Get(table Table, key []byte) ([]byte, error)

	PrefixIterator(table Table, prefix []byte) (Iterator, error)
}

type WriteBatch interface {
	PutBatch(table Table, entries []KV) error

	DeleteBatch(table Table, keys [][]byte) error

	Commit() error
}

type Iterator interface {
	Next() bool

	Key() []byte

	Value() []byte

	// Err reports why iteration stopped, or nil if it reached the end. Next
	// returning false means "no more entries" *or* "iteration failed", and a
	// caller that treats those the same silently accepts a partial scan as a
	// complete one.
	Err() error

	Close()
}

type KV struct {
	Key   []byte
	Value []byte
}
