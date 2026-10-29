package app

import (
	"bytes"
	"errors"
	"log"
	"maps"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/dataplane/fault"
	"github.com/HAL-X9/aegis/internal/dataplane/proxy"
	"github.com/HAL-X9/aegis/internal/observe/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func TestFailureReporter(t *testing.T) {
	var logged bytes.Buffer
	captureLog(t, &logged)

	registry := prometheus.NewRegistry()
	reporter := failureReporter{metrics: metrics.NewMetrics(registry)}
	r := httptest.NewRequest("GET", "/api/users", nil)
	r.Header.Set("X-Request-Id", "req-1")

	reporter.Failed(r, proxy.Failure{Reason: fault.NoRoute})
	if logged.Len() != 0 {
		t.Errorf("client error was logged: %q", logged.String())
	}

	reporter.Failed(r, proxy.Failure{Reason: fault.UpstreamFailed, Route: "api", Cause: errors.New("connection refused")})
	for _, want := range []string{"reason=upstream-failed", `route="api"`, `path="/api/users"`, `request_id="req-1"`, "cause=connection refused"} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("log %q lacks %q", logged.String(), want)
		}
	}

	want := map[string]float64{"no-route": 1, "upstream-failed": 1}
	if got := errorCounts(t, registry); !maps.Equal(got, want) {
		t.Errorf("aegis_http_errors_total = %v, want %v", got, want)
	}
}

func TestFailureReporterWithoutMetrics(t *testing.T) {
	captureLog(t, &bytes.Buffer{})

	// A disabled metrics middleware leaves the collector nil; reporting must
	// still work.
	failureReporter{}.Failed(httptest.NewRequest("GET", "/", nil), proxy.Failure{Reason: fault.UpstreamTimeout})
}

// captureLog redirects the standard logger to w for the rest of the test.
func captureLog(t *testing.T, w *bytes.Buffer) {
	t.Helper()
	previous := log.Writer()
	log.SetOutput(w)
	t.Cleanup(func() { log.SetOutput(previous) })
}

// errorCounts returns aegis_http_errors_total by reason.
func errorCounts(t *testing.T, registry *prometheus.Registry) map[string]float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	counts := make(map[string]float64)
	for _, family := range families {
		if family.GetName() != "aegis_http_errors_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			counts[metric.GetLabel()[0].GetValue()] = metric.GetCounter().GetValue()
		}
	}
	return counts
}
