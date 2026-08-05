package p2p

import (
	"github.com/libp2p/go-libp2p/core/network"

	"github.com/geanlabs/gean/internal/logger"
	"github.com/geanlabs/gean/internal/metrics"
)

// writeResponse arms the idle write deadline before every response frame so a peer
// that stops reading cannot hold an inbound handler open indefinitely.
func writeResponse(s network.Stream, label string, code byte, data []byte) bool {
	armWriteDeadline(s)
	encoded := EncodeResponse(code, data)
	metrics.ObserveReqRespResponseChunkSize(label, len(encoded))
	if _, err := s.Write(encoded); err != nil {
		if isStreamTimeout(err) {
			metrics.IncReqRespTimeout(label, "write")
		}
		logger.Warn(logger.Network, "%s: write response failed: %v", label, err)
		return false
	}
	return true
}
