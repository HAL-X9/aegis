package public

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HAL-X9/aegis/internal/config"
	"github.com/HAL-X9/aegis/internal/observe/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type okExecutor struct{}

func (okExecutor) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func testRuntime(mw config.Middleware) *config.Runtime {
	return &config.Runtime{
		Listeners:  config.Listeners{Public: config.HTTP{Addr: ":0"}},
		Middleware: mw,
	}
}

func TestPublicServerMiddlewareToggles(t *testing.T) {
	tests := map[string]config.Middleware{
		"all enabled":         {Metrics: true, RequestID: true},
		"metrics only":        {Metrics: true},
		"request id only":     {RequestID: true},
		"everything disabled": {},
	}

	for name, mw := range tests {
		t.Run(name, func(t *testing.T) {
			reg := prometheus.NewRegistry()

			var collector *metrics.Metrics
			if mw.Metrics {
				collector = metrics.NewMetrics(reg)
			}

			srv, err := NewPublicServer(testRuntime(mw), okExecutor{}, collector)
			if err != nil {
				t.Fatalf("NewPublicServer: %v", err)
			}

			rec := httptest.NewRecorder()
			srv.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			if got := rec.Header().Get("X-Request-Id") != ""; got != mw.RequestID {
				t.Errorf("X-Request-Id present = %v, want %v", got, mw.RequestID)
			}

			if got := recordedRequests(t, reg); got != mw.Metrics {
				t.Errorf("request recorded in metrics = %v, want %v", got, mw.Metrics)
			}
		})
	}
}

func TestPublicServerRequiresCollectorWhenMetricsEnabled(t *testing.T) {
	_, err := NewPublicServer(testRuntime(config.Middleware{Metrics: true}), okExecutor{}, nil)
	if err == nil {
		t.Fatal("NewPublicServer accepted a nil collector with metrics enabled, want an error")
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
