package p2p

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"

	"github.com/geanlabs/gean/internal/logger"
)

const statusMessageSize = 80

type StatusMessage struct {
	FinalizedRoot [32]byte
	FinalizedSlot uint64
	HeadRoot      [32]byte
	HeadSlot      uint64
}

func (s *StatusMessage) MarshalSSZ() []byte {
	buf := make([]byte, statusMessageSize)
	copy(buf[0:32], s.FinalizedRoot[:])
	binary.LittleEndian.PutUint64(buf[32:40], s.FinalizedSlot)
	copy(buf[40:72], s.HeadRoot[:])
	binary.LittleEndian.PutUint64(buf[72:80], s.HeadSlot)
	return buf
}

func (s *StatusMessage) UnmarshalSSZ(buf []byte) error {
	if len(buf) != statusMessageSize {
		return fmt.Errorf("status message has %d bytes, want %d", len(buf), statusMessageSize)
	}
	copy(s.FinalizedRoot[:], buf[0:32])
	s.FinalizedSlot = binary.LittleEndian.Uint64(buf[32:40])
	copy(s.HeadRoot[:], buf[40:72])
	s.HeadSlot = binary.LittleEndian.Uint64(buf[72:80])
	return nil
}

func handleStatusRequest(stream network.Stream, statusFn func() *StatusMessage) {
	armReadDeadline(stream)
	reqBuf, err := io.ReadAll(io.LimitReader(stream, int64(MaxCompressedPayloadSize)))
	if err != nil {
		logger.Warn(logger.Network, "status: read request failed: %v", err)
		return
	}
	if len(reqBuf) == 0 {
		writeResponse(stream, "status", RespInvalidRequest, []byte("empty status request"))
		return
	}

	payload, err := DecodeReqRespPayload(reqBuf)
	if err != nil {
		logger.Warn(logger.Network, "status: decode request failed: %v", err)
		writeResponse(stream, "status", RespInvalidRequest, []byte("decode failed"))
		return
	}

	peerStatus := &StatusMessage{}
	if err := peerStatus.UnmarshalSSZ(payload); err != nil {
		logger.Warn(logger.Network, "status: ssz unmarshal failed: %v", err)
		writeResponse(stream, "status", RespInvalidRequest, []byte("ssz unmarshal failed"))
		return
	}

	logger.Info(logger.Network, "status: peer at slot %d finalized=%d", peerStatus.HeadSlot, peerStatus.FinalizedSlot)

	status := statusFn()
	if status == nil {
		writeResponse(stream, "status", RespServerError, []byte("status unavailable"))
		return
	}
	writeResponse(stream, "status", RespSuccess, status.MarshalSSZ())
}

func (h *Host) SendStatusRequest(ctx context.Context, peerID peer.ID, ourStatus *StatusMessage) (*StatusMessage, error) {
	if ourStatus == nil {
		return nil, fmt.Errorf("status request: nil local status")
	}

	ctx, cancel := context.WithTimeout(ctx, ReqRespTimeout)
	defer cancel()

	stream, err := h.host.NewStream(ctx, peerID, protocol.ID(StatusProtocol))
	if err != nil {
		return nil, fmt.Errorf("open status stream: %w", err)
	}
	defer stream.Close()

	armWriteDeadline(stream)
	if _, err := stream.Write(EncodeReqRespPayload(ourStatus.MarshalSSZ())); err != nil {
		return nil, fmt.Errorf("write status request: %w", err)
	}
	stream.CloseWrite()

	code, respData, err := readReqRespChunk(stream, stream, "status")
	if err != nil {
		return nil, fmt.Errorf("read status response: %w", err)
	}
	if code != RespSuccess {
		return nil, fmt.Errorf("status response error: code=%d", code)
	}

	peerStatus := &StatusMessage{}
	if err := peerStatus.UnmarshalSSZ(respData); err != nil {
		return nil, fmt.Errorf("unmarshal status: %w", err)
	}
	return peerStatus, nil
}
