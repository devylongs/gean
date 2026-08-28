package p2p

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/types"
)

func handleBlocksByRangeRequest(
	stream network.Stream,
	currentSlotFn func() uint64,
	blocksInRangeFn func(startSlot, count uint64) ([]*types.SignedBlock, bool),
) {
	armReadDeadline(stream)
	reqBuf, err := io.ReadAll(io.LimitReader(stream, int64(MaxCompressedPayloadSize)))
	if err != nil {
		logger.Warn(logger.Network, "blocks_by_range: read request failed: %v", err)
		return
	}

	payload, err := DecodeReqRespPayload(reqBuf)
	if err != nil {
		logger.Error(logger.Network, "blocks_by_range: decode request failed: %v", err)
		writeResponse(stream, "blocks_by_range", RespInvalidRequest, []byte("decode failed"))
		return
	}

	req := &types.BlocksByRangeRequest{}
	if err := req.UnmarshalSSZ(payload); err != nil {
		logger.Error(logger.Network, "blocks_by_range: ssz unmarshal failed: %v", err)
		writeResponse(stream, "blocks_by_range", RespInvalidRequest, []byte("ssz unmarshal failed"))
		return
	}
	if err := validateRangeRequest(req.StartSlot, req.Count); err != nil {
		writeResponse(stream, "blocks_by_range", RespInvalidRequest, []byte("invalid count"))
		return
	}

	currentSlot := currentSlotFn()
	historyFloor := uint64(0)
	if currentSlot > types.MinSlotsForBlockRequests {
		historyFloor = currentSlot - types.MinSlotsForBlockRequests
	}
	if req.StartSlot < historyFloor {
		writeResponse(stream, "blocks_by_range", RespResourceUnavailable, []byte("start_slot below history horizon"))
		return
	}

	logger.Info(logger.Network, "blocks_by_range: peer requested start_slot=%d count=%d current_slot=%d",
		req.StartSlot, req.Count, currentSlot)

	blocks, complete := blocksInRangeFn(req.StartSlot, req.Count)
	// An incomplete ancestor chain is not an empty range. Serving it as a clean
	// empty stream tells the requester this range does not exist, so it stops
	// instead of asking a peer that can answer.
	if !complete {
		writeResponse(stream, "blocks_by_range", RespResourceUnavailable, []byte("history below stored chain"))
		return
	}

	for _, block := range blocks {
		if block == nil || block.Block == nil {
			continue
		}
		blockData, err := block.MarshalSSZ()
		if err != nil {
			logger.Warn(logger.Network, "blocks_by_range: marshal block at slot %d failed: %v", block.Block.Slot, err)
			if !writeResponse(stream, "blocks_by_range", RespServerError, []byte("marshal failed")) {
				return
			}
			continue
		}
		if !writeResponse(stream, "blocks_by_range", RespSuccess, blockData) {
			return
		}
		logger.Info(logger.Network, "blocks_by_range: served slot=%d", block.Block.Slot)
	}
}

func (h *Host) FetchBlocksByRange(
	ctx context.Context,
	peerID peer.ID,
	startSlot, count uint64,
) ([]*types.SignedBlock, error) {
	if err := validateRangeRequest(startSlot, count); err != nil {
		return nil, err
	}
	endSlot := startSlot + count

	ctx, cancel := context.WithTimeout(ctx, ReqRespTimeout)
	defer cancel()

	stream, err := h.host.NewStream(ctx, peerID, protocol.ID(BlocksByRangeProtocol))
	if err != nil {
		return nil, fmt.Errorf("open blocks_by_range stream: %w", err)
	}
	defer stream.Close()

	req := &types.BlocksByRangeRequest{StartSlot: startSlot, Count: count}
	reqSSZ, err := req.MarshalSSZ()
	if err != nil {
		return nil, fmt.Errorf("marshal blocks_by_range request: %w", err)
	}
	armWriteDeadline(stream)
	if _, err := stream.Write(EncodeReqRespPayload(reqSSZ)); err != nil {
		return nil, fmt.Errorf("write blocks_by_range request: %w", err)
	}
	stream.CloseWrite()

	var blocks []*types.SignedBlock
	var prevHeaderRoot [32]byte
	var hasPrev bool

	reader := bufio.NewReader(io.LimitReader(stream, int64(MaxCompressedPayloadSize)*int64(count)))
	for {
		code, blockData, err := readReqRespChunk(stream, reader, "blocks_by_range")
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode blocks_by_range response: %w", err)
		}
		if code == RespResourceUnavailable {
			return nil, fmt.Errorf("blocks_by_range: peer returned RESOURCE_UNAVAILABLE")
		}
		if code != RespSuccess {
			return nil, fmt.Errorf("blocks_by_range: peer returned error code %d", code)
		}

		block := &types.SignedBlock{}
		if err := block.UnmarshalSSZ(blockData); err != nil {
			return nil, fmt.Errorf("unmarshal block: %w", err)
		}
		if block.Block == nil || block.Block.Body == nil {
			return nil, fmt.Errorf("blocks_by_range: malformed block")
		}
		if block.Block.Slot < startSlot || block.Block.Slot >= endSlot {
			return nil, fmt.Errorf("blocks_by_range: block slot %d outside requested range [%d, %d)",
				block.Block.Slot, startSlot, endSlot)
		}

		if hasPrev {
			prevSlot := blocks[len(blocks)-1].Block.Slot
			if block.Block.Slot <= prevSlot {
				return nil, fmt.Errorf("blocks_by_range: non-monotonic slots %d <= %d",
					block.Block.Slot, prevSlot)
			}
			if block.Block.ParentRoot != prevHeaderRoot {
				return nil, fmt.Errorf("blocks_by_range: parent_root mismatch at slot %d (got 0x%x, want 0x%x)",
					block.Block.Slot, block.Block.ParentRoot, prevHeaderRoot)
			}
		}

		bodyRoot, err := block.Block.Body.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("body hash_tree_root: %w", err)
		}
		header := &types.BlockHeader{
			Slot:          block.Block.Slot,
			ProposerIndex: block.Block.ProposerIndex,
			ParentRoot:    block.Block.ParentRoot,
			StateRoot:     block.Block.StateRoot,
			BodyRoot:      bodyRoot,
		}
		prevHeaderRoot, err = header.HashTreeRoot()
		if err != nil {
			return nil, fmt.Errorf("header hash_tree_root: %w", err)
		}
		hasPrev = true

		blocks = append(blocks, block)
	}

	return blocks, nil
}

func validateRangeRequest(startSlot, count uint64) error {
	if count == 0 {
		return fmt.Errorf("blocks_by_range: count must be > 0")
	}
	if count > types.MaxRequestBlocks {
		return fmt.Errorf("blocks_by_range: count %d > MaxRequestBlocks %d", count, types.MaxRequestBlocks)
	}
	if startSlot > math.MaxUint64-count {
		return fmt.Errorf("blocks_by_range: startSlot+count overflows uint64")
	}
	return nil
}
