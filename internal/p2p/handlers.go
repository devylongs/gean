package p2p

import (
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/geanlabs/gean/internal/types"
)

func (h *Host) RegisterReqRespHandlers(
	statusFn func() *StatusMessage,
	blockByRootFn func(root [32]byte) *types.SignedBlock,
	currentSlotFn func() uint64,
	blocksInRangeFn func(startSlot, count uint64) ([]*types.SignedBlock, bool),
) {
	h.host.SetStreamHandler(protocol.ID(StatusProtocol), func(s network.Stream) {
		defer s.Close()
		handleStatusRequest(s, statusFn)
	})

	h.host.SetStreamHandler(protocol.ID(BlocksByRootProtocol), func(s network.Stream) {
		defer s.Close()
		handleBlocksByRootRequest(s, blockByRootFn)
	})

	h.host.SetStreamHandler(protocol.ID(BlocksByRangeProtocol), func(s network.Stream) {
		defer s.Close()
		handleBlocksByRangeRequest(s, currentSlotFn, blocksInRangeFn)
	})
}
