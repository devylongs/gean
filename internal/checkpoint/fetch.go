package checkpoint

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/geanlabs/gean/internal/logger"
)

const (
	// checkpointRequestTimeout bounds a single HTTP attempt.
	checkpointRequestTimeout = 30 * time.Second
	checkpointInitialBackoff = 1 * time.Second
	checkpointMaxBackoff     = 8 * time.Second
	checkpointMaxSSZBytes    = 64 << 20
)

// checkpointFetchBudget bounds the whole anchor fetch across retries, shared by the
// state and block requests. A checkpoint source can be briefly unready when this node
// boots against it; retrying rides that out instead of exiting. Kept well under the
// hive client-start window so a genuinely-down source still fails the node fast rather
// than stranding it.
var checkpointFetchBudget = 40 * time.Second

// checkpointHTTPClient carries no timeout of its own; each attempt is bounded by a
// per-request context so the shared budget governs the total.
var checkpointHTTPClient = &http.Client{}

// fetchSSZ retrieves an SSZ body, retrying transient failures with capped exponential
// backoff until the shared deadline. A finalized checkpoint is immutable, so a failed
// attempt is safe to repeat: it is either an endpoint still warming up (retry helps) or
// a misconfigured source (retry exhausts the budget and surfaces the last error).
func fetchSSZ(url string, deadline time.Time) ([]byte, error) {
	backoff := checkpointInitialBackoff
	var lastErr error
	for attempt := 1; ; attempt++ {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}

		body, err := fetchSSZOnce(url, minDuration(checkpointRequestTimeout, remaining))
		if err == nil {
			return body, nil
		}
		lastErr = err

		if time.Until(deadline) <= backoff {
			break
		}
		logger.Warn(logger.Sync, "checkpoint fetch attempt %d for %s failed: %v; retrying in %s",
			attempt, url, err, backoff)
		time.Sleep(backoff)
		if backoff < checkpointMaxBackoff {
			backoff *= 2
		}
	}
	return nil, fmt.Errorf("no successful response within %s: %w", checkpointFetchBudget, lastErr)
}

func fetchSSZOnce(url string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := checkpointHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d for %s", resp.StatusCode, url)
	}

	body, err := readLimitedSSZ(resp.Body, checkpointMaxSSZBytes)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return body, nil
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func readLimitedSSZ(r io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("checkpoint response exceeds %d bytes", maxBytes)
	}
	return body, nil
}
