package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HAL-X9/aegis/internal/dataplane/fault"
	"github.com/HAL-X9/aegis/internal/dataplane/policy"
	"github.com/HAL-X9/aegis/internal/httpmethod"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

type recordingObserver struct{ failures []Failure }

func (o *recordingObserver) Failed(_ *http.Request, failure Failure) {
	o.failures = append(o.failures, failure)
}

func TestExecutorAnswersEveryFailureWithItsReason(t *testing.T) {
	refused := errors.New("connection refused")

	tests := []struct {
		name        string
		unpublished bool
		method      string
		target      string
		upstreamErr error
		want        Failure
	}{
		{name: "no view", unpublished: true, method: http.MethodGet, target: "/api",
			want: Failure{Reason: fault.NotReady}},
		{name: "encoded slash", method: http.MethodGet, target: "/api%2Fx",
			want: Failure{Reason: fault.InvalidPath}},
		{name: "unsupported method", method: "TRACE", target: "/api",
			want: Failure{Reason: fault.UnsupportedMethod}},
		{name: "no route", method: http.MethodGet, target: "/other",
			want: Failure{Reason: fault.NoRoute}},
		{name: "method not allowed", method: http.MethodPost, target: "/api",
			want: Failure{Reason: fault.MethodNotAllowed}},
		{name: "rate limited", method: http.MethodGet, target: "/limited",
			want: Failure{Reason: fault.RateLimited, Route: "limited"}},
		{name: "upstream timeout", method: http.MethodGet, target: "/api", upstreamErr: timeoutError{},
			want: Failure{Reason: fault.UpstreamTimeout, Route: "api", Cause: timeoutError{}}},
		{name: "upstream failure", method: http.MethodGet, target: "/api", upstreamErr: refused,
			want: Failure{Reason: fault.UpstreamFailed, Route: "api", Cause: refused}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observer := &recordingObserver{}
			exec, err := NewExecutor(roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, tt.upstreamErr
			}), observer)
			if err != nil {
				t.Fatalf("NewExecutor: %v", err)
			}
			if !tt.unpublished {
				exec.Publish(failureTestView(t))
			}

			rec := httptest.NewRecorder()
			exec.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.target, nil))

			if rec.Code != tt.want.Reason.Status() {
				t.Errorf("status = %d, want %d", rec.Code, tt.want.Reason.Status())
			}
			if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", got)
			}
			if len(observer.failures) != 1 {
				t.Fatalf("observer saw %d failures, want 1", len(observer.failures))
			}
			got := observer.failures[0]
			if got.Reason != tt.want.Reason || got.Route != tt.want.Route || !errors.Is(got.Cause, tt.want.Cause) {
				t.Errorf("failure = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// failureTestView serves "api" (GET only) and "limited", whose rate limit
// rejects every request.
func failureTestView(t *testing.T) *View {
	t.Helper()

	engine := buildTestEngine(t, testConfig(
		snapshot.CompiledRoute{
			Name:     "api",
			Match:    snapshot.CompiledMatch{PathPrefix: "/api", Methods: httpmethod.GET},
			Policies: snapshot.CompiledRoutePolicies{RateLimitID: snapshot.NoRateLimit},
		},
		snapshot.CompiledRoute{
			Name:     "limited",
			Match:    snapshot.CompiledMatch{PathPrefix: "/limited", Methods: httpmethod.All},
			Policies: snapshot.CompiledRoutePolicies{RateLimitID: 0},
		},
	))

	return &View{
		Engine:   engine,
		Limiters: policy.NewRateLimiterSet([]snapshot.CompiledRateLimit{{Limit: 0, Burst: 0}}),
	}
}
