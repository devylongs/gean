package node

import (
	"context"
	"time"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/p2p"
)

const fetchBatchGracePeriod = 50 * time.Millisecond

func (e *Engine) runFetchBatcher(ctx context.Context) {
	for {
		var batch [][32]byte
		seen := make(map[[32]byte]bool)

		select {
		case <-ctx.Done():
			return
		case root := <-e.FetchRootCh:
			batch = append(batch, root)
			seen[root] = true
		}

		grace := time.After(fetchBatchGracePeriod)
	gather:
		for len(batch) < p2p.MaxBlocksPerRequest {
			select {
			case <-ctx.Done():
				return
			case root := <-e.FetchRootCh:
				if !seen[root] {
					batch = append(batch, root)
					seen[root] = true
				}
			case <-grace:
				break gather
			}
		}

		e.fireBatchFetch(ctx, batch)
	}
}

func (e *Engine) fireBatchFetch(ctx context.Context, roots [][32]byte) {
	if e.P2P == nil || len(roots) == 0 {
		return
	}
	logger.Info(logger.Sync, "batched fetch starting count=%d", len(roots))
	blocks, missing, err := e.P2P.FetchBlocksByRootBatchWithRetry(ctx, roots)
	if err != nil {
		logger.Warn(logger.Sync, "batched fetch failed count=%d err=%v", len(roots), err)
	}
	// Fetched parents fill gaps the dispatch loop is waiting on; deliver with
	// backpressure so none are dropped while their in-flight markers say the
	// fetch succeeded.
	for _, b := range blocks {
		if !e.OnSyncBlock(ctx, b) {
			return
		}
	}
	e.notifyFailedRoots(ctx, missing)
}

// notifyFailedRoots hands exhausted roots to the dispatch loop, blocking until each
// is accepted. Dropping one is not a lost log line: a root's in-flight marker clears
// only when its block arrives or this notification is handled, so a dropped root is
// never re-requested and its gap never closes. Range backfill cannot cover for it —
// that only runs against a peer whose head is ahead, which is untrue of a node
// marooned on its own fork. The marker map belongs to the dispatch loop, so the
// batcher cannot clear it here; backpressure is the only safe option, and the loop
// never sends on this channel, so blocking cannot deadlock.
func (e *Engine) notifyFailedRoots(ctx context.Context, roots [][32]byte) {
	for _, r := range roots {
		select {
		case e.FailedRootCh <- r:
		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) queueMissingBlockFetch(root [32]byte) {
	if e.P2P == nil {
		return
	}
	// A missing parent is re-derived on every child that arrives referencing it, so the
	// same root would otherwise be queued hundreds of times and saturate FetchRootCh with
	// duplicates — starving the fetch and freezing the head while far behind. Queue each
	// root at most once until its block is received or its fetch is exhausted.
	if e.fetchInFlight[root] {
		return
	}
	select {
	case e.FetchRootCh <- root:
		e.fetchInFlight[root] = true
		logger.Info(logger.Sync, "queueing missing block block_root=0x%x for batched fetch", root)
	default:
		logger.Warn(logger.Sync, "fetch root channel full, dropping request for 0x%x", root)
	}
}
