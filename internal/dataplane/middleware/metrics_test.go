package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HAL-X9/aegis/internal/observe/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// serveWithMetrics runs one request through the Metrics middleware around
// handler and returns the registry that recorded it.
func serveWithMetrics(t *testing.T, method string, handler http.HandlerFunc) *prometheus.Registry {
	t.Helper()

	reg := prometheus.NewRegistry()
	chain := NewMetricsMiddleware(metrics.NewMetrics(reg)).Metrics(handler)
	chain.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, "/", nil))
	return reg
}

// requestCount returns the aegis_http_requests_total sample with the given
// labels, or 0 if there is none.
func requestCount(t *testing.T, reg *prometheus.Registry, method, status string) float64 {
	t.Helper()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "aegis_http_requests_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := make(map[string]string)
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["method"] == method && labels["status"] == status {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func TestMetricsRecordsStatusAndMethod(t *testing.T) {
	tests := map[string]struct {
		method     string
		handler    http.HandlerFunc
		wantStatus string
	}{
		"explicit status": {
			method:     http.MethodPost,
			handler:    func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) },
			wantStatus: "418",
		},
		"body without WriteHeader is 200": {
			method:     http.MethodGet,
			handler:    func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("x")) },
			wantStatus: "200",
		},
		"empty handler is 200": {
			method:     http.MethodGet,
			handler:    func(http.ResponseWriter, *http.Request) {},
			wantStatus: "200",
		},
		"first WriteHeader wins": {
			method: http.MethodGet,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				w.WriteHeader(http.StatusOK)
			},
			wantStatus: "502",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			reg := serveWithMetrics(t, tc.method, tc.handler)

			if got := requestCount(t, reg, tc.method, tc.wantStatus); got != 1 {
				t.Fatalf("aegis_http_requests_total{method=%q,status=%q} = %v, want 1", tc.method, tc.wantStatus, got)
			}
		})
	}
}

func TestMetricsCountsEveryRequest(t *testing.T) {
	reg := prometheus.NewRegistry()
	chain := NewMetricsMiddleware(metrics.NewMetrics(reg)).Metrics(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	)

	for range 3 {
		chain.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}

	if got := requestCount(t, reg, http.MethodGet, "200"); got != 3 {
		t.Fatalf("request count = %v, want 3", got)
	}
}

// TestMetricsRecorderIsResetBetweenRequests guards the recorder pool: a status
// left over from one request must not leak into the next one served by the
// same pooled statusRecorder.
func TestMetricsRecorderIsResetBetweenRequests(t *testing.T) {
	reg := prometheus.NewRegistry()
	status := http.StatusInternalServerError
	chain := NewMetricsMiddleware(metrics.NewMetrics(reg)).Metrics(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if status != http.StatusOK {
				w.WriteHeader(status)
			}
		}),
	)

	chain.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	status = http.StatusOK
	chain.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if got := requestCount(t, reg, http.MethodGet, "500"); got != 1 {
		t.Errorf("status 500 count = %v, want 1", got)
	}
	if got := requestCount(t, reg, http.MethodGet, "200"); got != 1 {
		t.Errorf("status 200 count = %v, want 1 (a pooled recorder kept the previous status)", got)
	}
}
