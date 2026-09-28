// Package retry retries idempotent upstream requests after likely transient
// failures.
package retry

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"time"
)

const (
	baseBackoff = 25 * time.Millisecond
	maxBackoff  = 250 * time.Millisecond

	// maxDrain caps the bytes read from a discarded response so the
	// connection can be reused.
	maxDrain = 4 << 10
)

// Do perform rt.RoundTrip(req), making up to attempts total tries.
//
// It is deliberately tiny so the compiler inlines it: with retries disabled
// (attempts <= 1) the cost over a plain RoundTrip is one comparison.
func Do(rt http.RoundTripper, req *http.Request, attempts int) (*http.Response, error) {
	if attempts <= 1 {
		return rt.RoundTrip(req)
	}
	return roundTripWithRetries(rt, req, attempts)
}

// roundTripWithRetries repeats a failed attempt only when all of these hold:
//   - the method is idempotent (POST and PATCH are never repeated)
//   - the request body is empty (a server-side body cannot be replayed)
//   - the failure is a transport error or a 502/503/504 response
//   - the request context is still alive
//
// The last attempt's response or error is returned as-is, so the caller's
// error handling and streaming are unchanged. Backoff is exponential with
// full jitter and stops on context cancellation, so the listener's request
// timeout bounds the total retry time.
func roundTripWithRetries(rt http.RoundTripper, req *http.Request, attempts int) (*http.Response, error) {
	if !replayable(req) {
		return rt.RoundTrip(req)
	}

	for try := 1; ; try++ {
		resp, err := rt.RoundTrip(req)

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

func replayable(req *http.Request) bool {
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions,
		http.MethodTrace, http.MethodPut, http.MethodDelete:
		return req.Body == nil || req.Body == http.NoBody
	}
	return false
}

func retryable(ctx context.Context, resp *http.Response, err error) bool {
	if err != nil {
		return ctx.Err() == nil &&
			!errors.Is(err, context.Canceled) &&
			!errors.Is(err, context.DeadlineExceeded)
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
