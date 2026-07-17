package node

import (
	"context"
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
	Store               *store.ConsensusStore
	FC                  *forkchoice.ForkChoice
	P2P                 *p2p.Host
	Keys                *xmss.KeyManager
	AggCtl              *role.Controller
	DutyGate            *dutygate.Gate
	CommitteeCount      uint64
	Shadow              shadow.Rates
	Pending             *pending.BlockBuffer
	PendingAttestations *pending.AttestationBuffer

	BlockCh       chan *types.SignedBlock
	AttestationCh chan *types.SignedAttestation
	AggregationCh chan *types.SignedAggregatedAttestation
	FailedRootCh  chan [32]byte
	FetchRootCh   chan [32]byte

	AggregationDispatchCh chan aggregation.Dispatch
	ProposalCh            chan proposalDuty
	ProposalResultCh      chan *proposalResult
	RecoveryCh            chan *types.SignedBlock
	ProvingGate           *proving.Gate

	lastTick time.Time

	warnedMissingJustified [32]byte

	// fetchInFlight tracks block roots already queued for by-root fetch so a single
	// missing parent cannot flood FetchRootCh with duplicate requests. Accessed only
	// on the dispatch loop (queue on onBlock, clear on receive/exhaustion), so no lock.
	fetchInFlight map[[32]byte]bool
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
