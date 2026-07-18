package syncer

import (
	"context"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/types"
)

type SyncDriverP2P interface {
	Peers() []libp2ppeer.ID
	SendStatusRequest(ctx context.Context, peerID libp2ppeer.ID, ourStatus *p2p.StatusMessage) (*p2p.StatusMessage, error)
	FetchBlocksByRange(ctx context.Context, peerID libp2ppeer.ID, startSlot, count uint64) ([]*types.SignedBlock, error)
	FetchBlocksByRootBatchWithRetry(ctx context.Context, roots [][32]byte) ([]*types.SignedBlock, [][32]byte, error)
}

type LocalNode interface {
	GetSyncStatus() SyncStatus
	OnBlock(*types.SignedBlock)
	// OnSyncBlock delivers a requested block with backpressure; it blocks until
	// the node accepts it and returns false only if ctx ends first.
	OnSyncBlock(context.Context, *types.SignedBlock) bool
}
