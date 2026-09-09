package node

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/geanlabs/gean/internal/aggregation"
	"github.com/geanlabs/gean/internal/dutygate"
	"github.com/geanlabs/gean/internal/forkchoice"
	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/pending"
	"github.com/geanlabs/gean/internal/proving"
	"github.com/geanlabs/gean/internal/role"
	"github.com/geanlabs/gean/internal/shadow"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
	"github.com/geanlabs/gean/xmss"
)

var gitCommit = "unknown"

const (
	MaxBlockFetchDepth = 512
	MaxPendingBlocks   = 1024
)

const (
	PendingAttestationsPerRootCap = 8
	PendingAttestationsTotalCap   = 512
)

type Engine struct {
	Store          *store.ConsensusStore
	FC             *forkchoice.ForkChoice
	P2P            *p2p.Host
	Keys           *xmss.KeyManager
	AggCtl         *role.Controller
	DutyGate       *dutygate.Gate
	CommitteeCount uint64
	// AggregateSubnetIDs are the attestation subnets this node subscribes to as
	// an aggregator. Empty means every subnet. Set by the caller after New.
	AggregateSubnetIDs []uint64
	// expectedVoters caches how many validators this node can hear from in a
	// slot. Its inputs are fixed once the registry is known, and it is read on
	// every attestation arrival.
	expectedVoters      uint64
	Shadow              shadow.Rates
	Pending             *pending.BlockBuffer
	PendingAttestations *pending.AttestationBuffer

	BlockCh       chan *types.SignedBlock
	AttestationCh chan *types.SignedAttestation
	AggregationCh chan *types.SignedAggregatedAttestation
	FailedRootCh  chan [32]byte
	FetchRootCh   chan [32]byte

	// EarlyAggregateCh is a coalescing (capacity-1) wake-up: an attestation-verify
	// goroutine pokes it after inserting a signature, and the dispatch loop reacts
	// by considering an early aggregation session. It carries no data — the loop
	// re-reads live store state — so a full channel is simply dropped.
	EarlyAggregateCh chan struct{}

	AggregationDispatchCh chan aggregation.Dispatch
	ProposalCh            chan proposalDuty
	ProposalResultCh      chan *proposalResult
	RecoveryCh            chan *types.SignedBlock
	ProvingGate           *proving.Gate

	lastTick time.Time

	warnedMissingJustified [32]byte

	// maxSeenGossipSlot is the highest plausible slot heard on gossip, whether
	// or not the block was admitted. Written from the p2p goroutine, read on
	// the tick loop by the duty gate.
	maxSeenGossipSlot atomic.Uint64

	// fetchInFlight tracks block roots already queued for by-root fetch so a single
	// missing parent cannot flood FetchRootCh with duplicate requests. Accessed only
	// on the dispatch loop (queue on onBlock, clear on receive/exhaustion), so no lock.
	fetchInFlight  map[[32]byte]bool
	topicMeshSizes atomic.Pointer[map[string]int]

	// coveragePreMerge holds the new-payload participants captured before the
	// tick promoted them, keyed by the slot each vote is for. Read only on the
	// dispatch loop, which is also the only writer.
	coveragePreMerge map[uint64][][]byte

	// aggregatedSlot is the last slot for which an aggregation session was
	// dispatched, so the early (attestation-arrival) path and the interval-2
	// fallback dispatch at most once per slot. Written and read only on the
	// single dispatch loop, so no lock.
	aggregatedSlot uint64

	// numValidators caches the validator-set size, fixed at genesis in lean
	// devnet, used as the denominator for the early-aggregation quorum. Filled
	// lazily on the dispatch loop to avoid SSZ-decoding the head state on every
	// attestation arrival; read/written only there, so no lock.
	numValidators uint64
}

func New(
	s *store.ConsensusStore,
	fc *forkchoice.ForkChoice,
	p2pHost *p2p.Host,
	keys *xmss.KeyManager,
	aggCtl *role.Controller,
	committeeCount uint64,
	shadowRates shadow.Rates,
) *Engine {
	p2p.SetClientGitCommit(gitCommit)
	e := &Engine{
		Store:               s,
		FC:                  fc,
		P2P:                 p2pHost,
		Keys:                keys,
		AggCtl:              aggCtl,
		DutyGate:            dutygate.New(logDutyGateEvent),
		CommitteeCount:      committeeCount,
		Shadow:              shadowRates,
		Pending:             pending.NewBlockBuffer(),
		PendingAttestations: pending.NewAttestationBuffer(PendingAttestationsPerRootCap, PendingAttestationsTotalCap),
		// Sized so gossip keeps flowing while the dispatch loop chews through a
		// fetched batch: sync delivery blocks when full, but gossip drops, and a
		// large import burst can take minutes of XMSS verification.
		BlockCh:               make(chan *types.SignedBlock, 256),
		AttestationCh:         make(chan *types.SignedAttestation, 256),
		AggregationCh:         make(chan *types.SignedAggregatedAttestation, 64),
		FailedRootCh:          make(chan [32]byte, 64),
		FetchRootCh:           make(chan [32]byte, 256),
		EarlyAggregateCh:      make(chan struct{}, 1),
		AggregationDispatchCh: make(chan aggregation.Dispatch, 1),
		ProposalCh:            make(chan proposalDuty, 1),
		ProposalResultCh:      make(chan *proposalResult, 1),
		RecoveryCh:            make(chan *types.SignedBlock, 8),
		ProvingGate:           proving.NewGate(),
		fetchInFlight:         make(map[[32]byte]bool),
	}
	e.configureP2PHooks()
	return e
}

func (e *Engine) Run(ctx context.Context) {
	e.initMetrics()

	ticker := time.NewTicker(types.MillisecondsPerInterval * time.Millisecond)
	defer ticker.Stop()

	e.startWorkers(ctx)

	logger.Info(logger.Node, "started")
	e.onTick()
	e.dispatch(ctx, ticker.C)
}
