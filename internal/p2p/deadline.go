package p2p

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/libp2p/go-libp2p/core/network"

	"github.com/geanlabs/gean/internal/metrics"
)

// Req/resp streams are bounded by an idle deadline, reset before every read and
// write: a peer making progress is never cut, one that stalls is aborted within
// ReqRespTimeout. A single total deadline would instead sever a slow-but-valid
// large backfill mid-transfer. The context passed to NewStream covers only stream
// establishment, so without these the response read is unbounded and one stalled
// peer can wedge the fetch loop indefinitely.

func armReadDeadline(s network.Stream) {
	_ = s.SetReadDeadline(time.Now().Add(ReqRespTimeout))
}

func armWriteDeadline(s network.Stream) {
	_ = s.SetWriteDeadline(time.Now().Add(ReqRespTimeout))
}

// isStreamTimeout reports whether err is a read/write deadline expiry, so a stalled
// peer is attributed distinctly from other transport failures.
func isStreamTimeout(err error) bool {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// readReqRespChunk arms the idle read deadline, decodes one response frame, and on a
// deadline expiry records the stall and returns a clear error.
func readReqRespChunk(s network.Stream, r io.Reader, protocol string) (byte, []byte, error) {
	armReadDeadline(s)
	code, data, err := DecodeResponse(r)
	if err != nil && isStreamTimeout(err) {
		metrics.IncReqRespTimeout(protocol, "read")
		return code, data, fmt.Errorf("%s: response read stalled beyond %s: %w", protocol, ReqRespTimeout, err)
	}
	return code, data, err
}
