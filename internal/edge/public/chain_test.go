package public

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HAL-X9/aegis/internal/observe/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type okExecutor struct{}

func (okExecutor) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func TestHandlerMiddlewareToggles(t *testing.T) {
	tests := map[string]struct{ requestID, metrics bool }{
		"all enabled":         {requestID: true, metrics: true},
		"metrics only":        {metrics: true},
		"request id only":     {requestID: true},
		"everything disabled": {},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			reg := prometheus.NewRegistry()

			opts := Options{RequestID: tc.requestID}
			if tc.metrics {
				opts.Metrics = metrics.NewMetrics(reg)
			}

			rec := httptest.NewRecorder()
			NewHandler(okExecutor{}, opts).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			if got := rec.Header().Get("X-Request-Id") != ""; got != tc.requestID {
				t.Errorf("X-Request-Id present = %v, want %v", got, tc.requestID)
			}
			if got := recordedRequests(t, reg); got != tc.metrics {
				t.Errorf("request recorded in metrics = %v, want %v", got, tc.metrics)
			}
		})
	}
}

type deadlineExecutor struct{ deadline *time.Time }

func (e deadlineExecutor) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	if d, ok := r.Context().Deadline(); ok {
		*e.deadline = d
	}
}

func TestHandlerAppliesRequestTimeout(t *testing.T) {
	tests := map[string]struct {
		timeout     time.Duration
		wantDeadine bool
	}{
		"positive timeout sets a deadline": {timeout: time.Minute, wantDeadine: true},
		"zero timeout leaves it unbounded": {timeout: 0},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var deadline time.Time
			handler := NewHandler(deadlineExecutor{&deadline}, Options{RequestTimeout: tc.timeout})

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

			if got := !deadline.IsZero(); got != tc.wantDeadine {
				t.Fatalf("request context has a deadline = %v, want %v", got, tc.wantDeadine)
			}
		})
	}
}

// recordedRequests reports whether reg holds at least one recorded sample of
// aegis_http_requests_total.
func recordedRequests(t *testing.T, reg *prometheus.Registry) bool {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() == "aegis_http_requests_total" {
			return len(f.GetMetric()) > 0
		}
	}
	return false
}
