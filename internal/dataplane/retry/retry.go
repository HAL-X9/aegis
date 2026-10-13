// Package retry retries idempotent upstream requests after likely transient
// failures.
package retry

import (
	"context"
	"io"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

const (
	baseBackoff = 25 * time.Millisecond
	maxBackoff  = 250 * time.Millisecond

	// maxDrain caps the bytes read from a discarded response so the
	// connection can be reused.
	maxDrain = 4 << 10
)

// Do sends req through rt under policy: up to policy.Attempts tries, each
// bounded by policy.AttemptTimeout if it is positive.
//
// With a single attempt and no attempt timeout, Do is a plain RoundTrip.
func Do(rt http.RoundTripper, req *http.Request, policy snapshot.CompiledRetry) (*http.Response, error) {
	if policy.Attempts <= 1 && policy.AttemptTimeout <= 0 {
		return rt.RoundTrip(req)
	}
	return roundTripWithRetries(rt, req, policy)
}

// roundTripWithRetries repeats a failed attempt only when all of these hold:
//   - the method is idempotent (POST and PATCH are never repeated)
//   - the request body is empty (a server-side body cannot be replayed)
//   - the failure is a transport error, an expired attempt timeout, or a
//     502/503/504 response
//   - the request context is still alive
//
// The last attempt's response or error is returned as-is. Backoff is
// exponential with full jitter and stops on context cancellation, so the
// request context bounds the total retry time.
func roundTripWithRetries(rt http.RoundTripper, req *http.Request, policy snapshot.CompiledRetry) (*http.Response, error) {
	attempts := policy.Attempts
	if !replayable(req) {
		attempts = 1
	}

	for try := 1; ; try++ {
		resp, err := attempt(rt, req, policy.AttemptTimeout)

		if try >= attempts || !retryable(req.Context(), resp, err) {
			return resp, err
		}

		if resp != nil {
			_, _ = io.CopyN(io.Discard, resp.Body, maxDrain)
			_ = resp.Body.Close()
		}

		if err := sleep(req.Context(), backoff(try)); err != nil {
			return nil, err
		}
	}
}

// attempt sends req once. A positive timeout bounds the attempt until its
// response body is closed.
func attempt(rt http.RoundTripper, req *http.Request, timeout time.Duration) (*http.Response, error) {
	if timeout <= 0 {
		return rt.RoundTrip(req)
	}

	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	resp, err := rt.RoundTrip(req.WithContext(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// cancelOnClose releases an attempt's context once its body is closed.
type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b cancelOnClose) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func replayable(req *http.Request) bool {
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions,
		http.MethodTrace, http.MethodPut, http.MethodDelete:
		return req.Body == nil || req.Body == http.NoBody
	}
	return false
}

// retryable reports whether an attempt's outcome warrants another try. An
// error is retryable while ctx, the request context, is alive; this
// includes an expired attempt timeout.
func retryable(ctx context.Context, resp *http.Response, err error) bool {
	if err != nil {
		return ctx.Err() == nil
	}
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

func backoff(try int) time.Duration {
	d := baseBackoff << (try - 1)
	if d > maxBackoff || d <= 0 {
		d = maxBackoff
	}
	return time.Duration(rand.Int64N(int64(d))) + time.Millisecond
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
