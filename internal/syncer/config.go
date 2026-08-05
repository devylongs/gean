package syncer

import "time"

const (
	blocksByRangeSyncThreshold = 2
	pollInterval               = 32 * time.Second
	peerStatusTimeout          = 10 * time.Second

	// forkReconcileFinalizedThreshold is how far our finalized checkpoint may
	// fall behind a peer's before we treat it as a fork we are marooned on and
	// range-backfill to reconcile. Cross-node finalized skew is normally 0-2
	// slots on a shared chain, so this stays clear of healthy operation.
	forkReconcileFinalizedThreshold = 8
)
