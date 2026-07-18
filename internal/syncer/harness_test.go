package syncer

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libp2ppeer "github.com/libp2p/go-libp2p/core/peer"

	"github.com/geanlabs/gean/internal/p2p"
	"github.com/geanlabs/gean/internal/storage"
	"github.com/geanlabs/gean/internal/store"
	"github.com/geanlabs/gean/internal/types"
)

type mockSyncP2P struct {
	mu sync.Mutex

	peers []libp2ppeer.ID

	statusResp *p2p.StatusMessage
	statusErr  error

	rangeBatches   [][]*types.SignedBlock
	rangeErr       error
	rangeCalls     atomic.Int32
	lastRangeCount uint64
	rangeBlock     chan struct{}

	rootBlocks []*types.SignedBlock
	rootErr    error
	rootCalls  atomic.Int32
}

func (m *mockSyncP2P) Peers() []libp2ppeer.ID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.peers
}

func (m *mockSyncP2P) SendStatusRequest(ctx context.Context, peerID libp2ppeer.ID, ourStatus *p2p.StatusMessage) (*p2p.StatusMessage, error) {
	return m.statusResp, m.statusErr
}

func (m *mockSyncP2P) FetchBlocksByRange(ctx context.Context, peerID libp2ppeer.ID, startSlot, count uint64) ([]*types.SignedBlock, error) {
	m.rangeCalls.Add(1)
	m.mu.Lock()
	m.lastRangeCount = count
	m.mu.Unlock()

	if m.rangeBlock != nil {
		<-m.rangeBlock
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.rangeBatches) == 0 {
		return nil, m.rangeErr
	}
	batch := m.rangeBatches[0]
	m.rangeBatches = m.rangeBatches[1:]
	return batch, m.rangeErr
}

func (m *mockSyncP2P) FetchBlocksByRootBatchWithRetry(ctx context.Context, roots [][32]byte) ([]*types.SignedBlock, [][32]byte, error) {
	m.rootCalls.Add(1)
	return m.rootBlocks, nil, m.rootErr
}

type testNode struct {
	status  SyncStatus
	BlockCh chan *types.SignedBlock
}

func (n *testNode) GetSyncStatus() SyncStatus { return n.status }

func (n *testNode) OnBlock(block *types.SignedBlock) {
	n.BlockCh <- block
}

func (n *testNode) OnSyncBlock(ctx context.Context, block *types.SignedBlock) bool {
	select {
	case n.BlockCh <- block:
		return true
	case <-ctx.Done():
		return false
	}
}

func makeTestSyncHarness() (*testNode, *store.ConsensusStore) {
	store := store.NewConsensusStore(storage.NewInMemoryBackend())
	header := &types.BlockHeader{Slot: 0}
	root := mustHeaderRoot(header)
	store.SetHead(root)
	store.InsertBlockHeader(root, header)
	return &testNode{status: SyncSyncing, BlockCh: make(chan *types.SignedBlock, 16)}, store
}

func makeSyncRange(parentRoot [32]byte, slots ...uint64) []*types.SignedBlock {
	blocks := make([]*types.SignedBlock, 0, len(slots))
	for _, slot := range slots {
		block, root := makeSyncBlock(slot, parentRoot)
		blocks = append(blocks, block)
		parentRoot = root
	}
	return blocks
}

func makeSyncBlock(slot uint64, parentRoot [32]byte) (*types.SignedBlock, [32]byte) {
	body := &types.BlockBody{}
	bodyRoot, err := body.HashTreeRoot()
	if err != nil {
		panic(err)
	}
	header := &types.BlockHeader{Slot: slot, ParentRoot: parentRoot, BodyRoot: bodyRoot}
	root := mustHeaderRoot(header)
	return &types.SignedBlock{
		Block: &types.Block{
			Slot:       slot,
			ParentRoot: parentRoot,
			Body:       body,
		},
		Proof: &types.MultiMessageAggregate{},
	}, root
}

func mustHeaderRoot(header *types.BlockHeader) [32]byte {
	root, err := header.HashTreeRoot()
	if err != nil {
		panic(err)
	}
	return root
}

func drainBlockCh(t *testing.T, n *testNode, expected int, timeout time.Duration) []*types.SignedBlock {
	t.Helper()
	var out []*types.SignedBlock
	deadline := time.After(timeout)
	for len(out) < expected {
		select {
		case b := <-n.BlockCh:
			out = append(out, b)
		case <-deadline:
			return out
		}
	}
	return out
}
