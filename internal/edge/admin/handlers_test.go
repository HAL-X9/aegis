package admin

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/observe/health"
)

type fakeProbe struct {
	live, ready error
}

func (p fakeProbe) Liveness() error  { return p.live }
func (p fakeProbe) Readiness() error { return p.ready }

func serve(t *testing.T, router http.Handler, method, target string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	body, _ := io.ReadAll(rec.Result().Body)
	return rec.Code, string(body)
}

func newRouter(probe Probe, metrics http.Handler) http.Handler {
	return NewRouter(NewSystemHandler(probe, metrics))
}

func TestProbesReportStatus(t *testing.T) {
	failure := errors.New("down")

	tests := map[string]struct {
		probe      fakeProbe
		path       string
		wantStatus int
		wantBody   string
	}{
		"alive":     {fakeProbe{}, "/livez", http.StatusOK, "ok\n"},
		"not alive": {fakeProbe{live: failure}, "/livez", http.StatusServiceUnavailable, "service not alive\n"},
		"ready":     {fakeProbe{}, "/readyz", http.StatusOK, "ready\n"},
		"not ready": {fakeProbe{ready: failure}, "/readyz", http.StatusServiceUnavailable, "service not ready\n"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			status, body := serve(t, newRouter(tc.probe, http.NotFoundHandler()), http.MethodGet, tc.path)
			if status != tc.wantStatus || body != tc.wantBody {
				t.Fatalf("GET %s = %d %q, want %d %q", tc.path, status, body, tc.wantStatus, tc.wantBody)
			}
		})
	}
}

// TestReadyzFollowsShutdown covers the contract the lifecycle relies on:
// readiness turns 503 as soon as shutdown begins, liveness stays green.
func TestReadyzFollowsShutdown(t *testing.T) {
	probe := health.NewHealth()
	router := newRouter(probe, http.NotFoundHandler())

	if status, _ := serve(t, router, http.MethodGet, "/readyz"); status != http.StatusOK {
		t.Fatalf("/readyz before shutdown = %d, want 200", status)
	}

	probe.SetShuttingDown(true)

	if status, _ := serve(t, router, http.MethodGet, "/readyz"); status != http.StatusServiceUnavailable {
		t.Errorf("/readyz during shutdown = %d, want 503", status)
	}
	if status, _ := serve(t, router, http.MethodGet, "/livez"); status != http.StatusOK {
		t.Errorf("/livez during shutdown = %d, want 200", status)
	}
}

func TestMetricsDelegatesToHandler(t *testing.T) {
	metrics := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "aegis_up 1\n")
	})

	status, body := serve(t, newRouter(fakeProbe{}, metrics), http.MethodGet, "/metrics")
	if status != http.StatusOK || !strings.Contains(body, "aegis_up 1") {
		t.Fatalf("GET /metrics = %d %q, want the metrics handler's response", status, body)
	}
}

func TestSystemRouterOnlyServesGET(t *testing.T) {
	router := newRouter(fakeProbe{}, http.NotFoundHandler())

	for _, path := range []string{"/livez", "/readyz", "/metrics"} {
		if status, _ := serve(t, router, http.MethodPost, path); status != http.StatusMethodNotAllowed {
			t.Errorf("POST %s = %d, want 405", path, status)
		}
	}
	if status, _ := serve(t, router, http.MethodGet, "/unknown"); status != http.StatusNotFound {
		t.Errorf("GET /unknown = %d, want 404", status)
	}
}
