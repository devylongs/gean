package p2p

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/types"
)

func handleBlocksByRootRequest(stream network.Stream, blockByRootFn func(root [32]byte) *types.SignedBlock) {
	armReadDeadline(stream)
	reqBuf, err := io.ReadAll(io.LimitReader(stream, int64(MaxCompressedPayloadSize)))
	if err != nil {
		logger.Warn(logger.Network, "blocks_by_root: read request failed: %v", err)
		return
	}

	payload, err := DecodeReqRespPayload(reqBuf)
	if err != nil {
		logger.Error(logger.Network, "blocks_by_root: decode request failed: %v", err)
		writeResponse(stream, "blocks_by_root", RespInvalidRequest, []byte("decode failed"))
		return
	}

	roots, err := DecodeBlocksByRootRequest(payload)
	if err != nil {
		logger.Error(logger.Network, "blocks_by_root: %v", err)
		writeResponse(stream, "blocks_by_root", RespInvalidRequest, []byte("ssz unmarshal failed"))
		return
	}
	if err := validateRootCount(len(roots)); err != nil {
		writeResponse(stream, "blocks_by_root", RespInvalidRequest, []byte("invalid root count"))
		return
	}

	logger.Info(logger.Network, "blocks_by_root: peer requested %d roots", len(roots))

	for _, root := range roots {
		block := blockByRootFn(root)
		if block == nil {
			logger.Info(logger.Network, "blocks_by_root: block not found root=0x%x", root)
			continue
		}

		blockData, err := block.MarshalSSZ()
		if err != nil {
			logger.Warn(logger.Network, "blocks_by_root: marshal failed root=0x%x: %v", root, err)
			if !writeResponse(stream, "blocks_by_root", RespServerError, []byte("marshal failed")) {
				return
			}
			continue
		}

		if !writeResponse(stream, "blocks_by_root", RespSuccess, blockData) {
			return
		}
		logger.Info(logger.Network, "blocks_by_root: served block slot=%d root=0x%x", block.Block.Slot, root)
	}
}

func (h *Host) FetchBlocksByRoot(ctx context.Context, peerID peer.ID, roots [][32]byte) ([]*types.SignedBlock, error) {
	if err := validateRootCount(len(roots)); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, ReqRespTimeout)
	defer cancel()

	stream, err := h.host.NewStream(ctx, peerID, protocol.ID(BlocksByRootProtocol))
	if err != nil {
		return nil, fmt.Errorf("open blocks_by_root stream: %w", err)
	}
	defer stream.Close()

	armWriteDeadline(stream)
	if _, err := stream.Write(EncodeReqRespPayload(EncodeBlocksByRootRequest(roots))); err != nil {
		return nil, fmt.Errorf("write blocks request: %w", err)
	}
	stream.CloseWrite()

	var blocks []*types.SignedBlock
	requestedRoots := requestedRootSet(roots)
	seenRoots := make(map[[32]byte]bool, len(roots))
	reader := bufio.NewReader(io.LimitReader(stream, int64(MaxCompressedPayloadSize)*int64(len(roots))))
	for {
		code, blockData, err := readReqRespChunk(stream, reader, "blocks_by_root")
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode blocks_by_root response: %w", err)
		}
		if code != RespSuccess || blockData == nil {
			return nil, fmt.Errorf("blocks_by_root: peer returned error code %d", code)
		}
		block := &types.SignedBlock{}
		if err := block.UnmarshalSSZ(blockData); err != nil {
			return nil, fmt.Errorf("unmarshal block: %w", err)
		}
		if block.Block == nil {
			return nil, fmt.Errorf("blocks_by_root: malformed block")
		}
		if err := validateFetchedBlockRoot(block, requestedRoots, seenRoots); err != nil {
			return nil, fmt.Errorf("blocks_by_root: %w", err)
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func requestedRootSet(roots [][32]byte) map[[32]byte]bool {
	set := make(map[[32]byte]bool, len(roots))
	for _, root := range roots {
		set[root] = true
	}
	return set
}

func validateFetchedBlockRoot(
	block *types.SignedBlock,
	requestedRoots map[[32]byte]bool,
	seenRoots map[[32]byte]bool,
) error {
	if block == nil || block.Block == nil {
		return fmt.Errorf("malformed block")
	}
	root, err := block.Block.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("compute block root: %w", err)
	}
	if !requestedRoots[root] {
		return fmt.Errorf("peer returned unrequested block root 0x%x", root)
	}
	if seenRoots[root] {
		return fmt.Errorf("peer returned duplicate block root 0x%x", root)
	}
	seenRoots[root] = true
	return nil
}

func EncodeBlocksByRootRequest(roots [][32]byte) []byte {
	rootsData := make([]byte, len(roots)*32)
	for i, root := range roots {
		copy(rootsData[i*32:], root[:])
	}

	container := make([]byte, 4+len(rootsData))
	binary.LittleEndian.PutUint32(container[:4], 4)
	copy(container[4:], rootsData)
	return container
}

func DecodeBlocksByRootRequest(payload []byte) ([][32]byte, error) {
	if len(payload) < 4 {
		return nil, fmt.Errorf("payload too short: %d bytes", len(payload))
	}

	offset := binary.LittleEndian.Uint32(payload[:4])
	if offset != 4 {
		return nil, fmt.Errorf("unexpected SSZ offset %d, expected 4", offset)
	}

	rootsData := payload[4:]
	if len(rootsData)%32 != 0 {
		return nil, fmt.Errorf("roots data size %d not multiple of 32", len(rootsData))
	}

	roots := make([][32]byte, len(rootsData)/32)
	for i := range roots {
		copy(roots[i][:], rootsData[i*32:(i+1)*32])
	}
	return roots, nil
}

func validateRootCount(count int) error {
	if count == 0 || count > int(types.MaxRequestBlocks) {
		return fmt.Errorf("blocks_by_root: root count %d outside range [1,%d]", count, types.MaxRequestBlocks)
	}
	return nil
}
