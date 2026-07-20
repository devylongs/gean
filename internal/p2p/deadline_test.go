package p2p

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
)

// deadlineStream records the deadlines set on it and serves a scripted read error,
// standing in for a peer that opens a req/resp stream and then stalls.
type deadlineStream struct {
	network.Stream
	readDeadline time.Time
	readErr      error
}

func (s *deadlineStream) SetReadDeadline(t time.Time) error  { s.readDeadline = t; return nil }
func (s *deadlineStream) SetWriteDeadline(t time.Time) error { return nil }
func (s *deadlineStream) Read(p []byte) (int, error) {
	if s.readErr != nil {
		return 0, s.readErr
	}
	return 0, io.EOF
}

// A stalled response read must be bounded by an idle deadline — before this fix the
// read had none and a connected-but-silent peer blocked the fetch loop forever.
func TestReadReqRespChunkArmsIdleDeadline(t *testing.T) {
	s := &deadlineStream{readErr: os.ErrDeadlineExceeded}
	before := time.Now()

	if _, _, err := readReqRespChunk(s, s, "test"); err == nil {
		t.Fatal("expected an error from a stalled read")
	}

	if s.readDeadline.Before(before.Add(ReqRespTimeout - time.Second)) {
		t.Fatalf("read deadline not armed to ~now+%s: got %v", ReqRespTimeout, s.readDeadline)
	}
}

// A deadline expiry is reported as a stall with a clear, timeout-classified error so
// the caller can rotate peers instead of hanging.
func TestReadReqRespChunkWrapsTimeout(t *testing.T) {
	s := &deadlineStream{readErr: os.ErrDeadlineExceeded}

	_, _, err := readReqRespChunk(s, s, "blocks_by_root")
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("timeout not preserved through wrapping: %v", err)
	}
	if !strings.Contains(err.Error(), "stalled") || !strings.Contains(err.Error(), "blocks_by_root") {
		t.Fatalf("error should name the protocol and the stall: %v", err)
	}
}

// A non-timeout transport error passes through unchanged, not misattributed as a stall.
func TestReadReqRespChunkPassesThroughNonTimeout(t *testing.T) {
	s := &deadlineStream{readErr: io.ErrUnexpectedEOF}

	_, _, err := readReqRespChunk(s, s, "test")
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "stalled") {
		t.Fatalf("non-timeout error misreported as a stall: %v", err)
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestIsStreamTimeout(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"deadline exceeded", os.ErrDeadlineExceeded, true},
		{"wrapped deadline", fmt.Errorf("read: %w", os.ErrDeadlineExceeded), true},
		{"net timeout", fmt.Errorf("read: %w", timeoutError{}), true},
		{"eof", io.EOF, false},
		{"nil", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isStreamTimeout(tc.err); got != tc.want {
				t.Fatalf("isStreamTimeout(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
