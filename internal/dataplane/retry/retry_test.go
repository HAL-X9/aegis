package retry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// hangThenRespond returns a transport whose first hangs attempts block until
// their context ends, and whose later attempts respond 200 "ok".
func hangThenRespond(hangs int, calls *int) http.RoundTripper {
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		*calls++
		if *calls <= hangs {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
}

func TestDoRetriesAfterAttemptTimeout(t *testing.T) {
	var calls int
	req := httptest.NewRequest(http.MethodGet, "http://upstream/", nil)
	policy := snapshot.CompiledRetry{Attempts: 3, AttemptTimeout: 10 * time.Millisecond}

	resp, err := Do(hangThenRespond(2, &calls), req, policy)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if calls != 3 {
		t.Fatalf("attempts = %d, want 3", calls)
	}
	// The final attempt's context must stay alive until the body is closed.
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "ok" {
		t.Fatalf("body = %q, %v; want %q", body, err, "ok")
	}
}

func TestDoReturnsDeadlineWhenEveryAttemptTimesOut(t *testing.T) {
	var calls int
	req := httptest.NewRequest(http.MethodGet, "http://upstream/", nil)
	policy := snapshot.CompiledRetry{Attempts: 2, AttemptTimeout: 10 * time.Millisecond}

	_, err := Do(hangThenRespond(2, &calls), req, policy)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if calls != 2 {
		t.Fatalf("attempts = %d, want 2", calls)
	}
}

func TestDoBoundsSingleAttempt(t *testing.T) {
	var calls int
	req := httptest.NewRequest(http.MethodGet, "http://upstream/", nil)
	policy := snapshot.CompiledRetry{Attempts: 1, AttemptTimeout: 10 * time.Millisecond}

	_, err := Do(hangThenRespond(1, &calls), req, policy)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestDoDoesNotRetryNonReplayableRequest(t *testing.T) {
	var calls int
	req := httptest.NewRequest(http.MethodPost, "http://upstream/", strings.NewReader("payload"))
	policy := snapshot.CompiledRetry{Attempts: 3, AttemptTimeout: 10 * time.Millisecond}

	if _, err := Do(hangThenRespond(1, &calls), req, policy); err == nil {
		t.Fatal("Do succeeded, want the attempt timeout")
	}
	if calls != 1 {
		t.Fatalf("attempts = %d, want 1", calls)
	}
}

func TestDoStopsWhenRequestContextEnds(t *testing.T) {
	var calls int
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "http://upstream/", nil).WithContext(ctx)
	policy := snapshot.CompiledRetry{Attempts: 5}

	_, err := Do(hangThenRespond(5, &calls), req, policy)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if calls != 1 {
		t.Fatalf("attempts = %d, want 1", calls)
	}
}
