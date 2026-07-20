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

// Req/resp streams are bounded by an idle deadline, not a single total one: the
// deadline is reset before every read and write, so a peer that keeps making
// progress is never cut, while one that opens the stream and then stalls is
// aborted within ReqRespTimeout. A total deadline would sever a legitimate but
// slow large backfill mid-transfer; the context passed to NewStream only covers
// stream establishment, so without these the response read is unbounded and a
// single stalled peer can wedge the fetch loop indefinitely.

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
// deadline expiry records the stall and returns a clear error. r reads from the stream
// (directly or through a size-limiting wrapper); s is armed for the deadline.
func readReqRespChunk(s network.Stream, r io.Reader, protocol string) (byte, []byte, error) {
	armReadDeadline(s)
	code, data, err := DecodeResponse(r)
	if err != nil && isStreamTimeout(err) {
		metrics.IncReqRespTimeout(protocol, "read")
		return code, data, fmt.Errorf("%s: response read stalled beyond %s: %w", protocol, ReqRespTimeout, err)
	}
	return code, data, err
}
