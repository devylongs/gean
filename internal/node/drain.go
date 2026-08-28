package node

import "github.com/geanlabs/gean/internal/logger"

// drainPendingBlocks imports the blocks already queued when the drain starts and
// no more. Sync delivery blocks on BlockCh, so a producer refills every slot the
// drain frees: draining until the channel reads empty hands the tick loop an
// unbounded amount of work, and store.OnTick has already run for this tick — the
// store clock then sits stale for the whole drain and correctly-timed blocks are
// rejected as beyond the future horizon. Whatever arrives mid-drain is picked up
// by the dispatch loop or the next tick.
func (e *Engine) drainPendingBlocks() int {
	drained := 0
drain:
	for remaining := len(e.BlockCh); remaining > 0; remaining-- {
		select {
		case block := <-e.BlockCh:
			e.onBlock(block)
			drained++
		default:
			// The dispatch loop took part of the batch first; nothing left.
			break drain
		}
	}
	if drained > 0 {
		logger.Info(logger.Chain, "drained %d pending blocks before attestation", drained)
	}
	return drained
}
