package p2p

import "time"

const (
	StatusProtocol        = "/leanconsensus/req/status/1/ssz_snappy"
	BlocksByRootProtocol  = "/leanconsensus/req/blocks_by_root/1/ssz_snappy"
	BlocksByRangeProtocol = "/leanconsensus/req/blocks_by_range/1/ssz_snappy"
)

// ReqRespTimeout is the idle deadline applied to each individual req/resp read and
// write (see deadline.go). It bounds how long a single stalled operation may block,
// not the whole exchange — a transfer that keeps making progress is never cut.
const ReqRespTimeout = 15 * time.Second
