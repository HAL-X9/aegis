// Package e2e_test exercises the gateway end-to-end: real YAML configuration
// files are loaded through the same config.Load/loader.Load path main.go
// uses, app.Bootstrap wires the full dependency graph, and requests are sent
// through the resulting http.Handler against a real mock upstream server.
//
// This intentionally bypasses app.New() (flag parsing) and Runtime.Run()
// (signal handling, real listener sockets) — those are process-lifecycle
// concerns, not request-handling behavior. Testing through the handler
// directly, via httptest.NewServer, exercises the identical routing/policy/
// proxy pipeline without binding real ports or depending on OS signals.
package e2e_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/HAL-X9/aegis/internal/app"
	"github.com/HAL-X9/aegis/internal/config"
	"github.com/HAL-X9/aegis/internal/controlplane/loader"
	"github.com/prometheus/client_golang/prometheus"
)

// newMockUpstream starts a real HTTP server standing in for a proxied
// service. /api/v1/profile returns a fixed JSON body and records the
// headers it received on capturedHeaders, so request-side policy mutations
// (add/remove) can be asserted against what the upstream actually got —
// not just inferred from the gateway's own response.
func newMockUpstream(t *testing.T) (srv *httptest.Server, capturedHeaders *http.Header) {
	t.Helper()

	captured := &http.Header{}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/profile", func(w http.ResponseWriter, r *http.Request) {
		*captured = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, captured
}

// writeTempFile writes contents to name inside t.TempDir() and returns the
// full path. Using real files (not in-memory structs) exercises the exact
// config.Load / loader.Load code path main.go uses in production, rather
// than constructing internal schema types by hand.
func writeTempFile(t *testing.T, dir, name, contents string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// bootstrapTestGateway loads a runtime config and a gateway manifest that
// routes to upstream, then bootstraps the full dependency graph via
// app.Bootstrap — the same call cmd/main.go makes, just with configuration
// pointed at a test upstream instead of production services.
func bootstrapTestGateway(t *testing.T, upstream *httptest.Server) *app.Dependencies {
	t.Helper()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	host := upstreamURL.Hostname()
	port, err := strconv.Atoi(upstreamURL.Port())
	if err != nil {
		t.Fatalf("parse upstream port: %v", err)
	}

	dir := t.TempDir()

	runtimeConfigYAML := `
listeners:
  public:
    addr: ":0"
    timeouts:
      read_timeout: 10s
      read_header_timeout: 5s
      write_timeout: 30s
      idle_timeout: 120s
    max_header_bytes: 1048576

  system:
    addr: "127.0.0.1:0"
    timeouts:
      read_timeout: 5s
      read_header_timeout: 2s
      write_timeout: 10s
      idle_timeout: 30s
    max_header_bytes: 262144

proxy:
  trusted_proxies:
    - 127.0.0.1/32

upstream_transport:
  max_idle_conns: 100
  max_idle_conns_per_host: 32
  max_conns_per_host: 0
  dial_timeout: 3s
  tls_handshake_timeout: 5s
  response_header_timeout: 10s
  idle_conn_timeout: 90s

observability:
  logging:
    level: info
    format: json
  tracing:
    enabled: false
    endpoint: "http://localhost:4318"
`
	runtimeConfigPath := writeTempFile(t, dir, "aegis.yaml", runtimeConfigYAML)

	gatewayConfigYAML := `
services:
  user-profile:
    upstream:
      scheme: http
      host: ` + host + `
      port: ` + strconv.Itoa(port) + `

routes:
  - name: user-profile
    service: user-profile
    match:
      path_prefix: /api/v1/profile
      methods: [GET]
    policies:
      - name: security-headers

policies:
  headers:
    security-headers:
      request:
        add:
          X-Request-Id: "e2e-test"
        remove:
          - X-Forwarded-For
      response:
        add:
          X-Content-Type-Options: "nosniff"
          X-Frame-Options: "DENY"
        remove:
          - Server
`
	gatewayConfigPath := writeTempFile(t, dir, "gateway.yaml", gatewayConfigYAML)

	runtimeConfig, err := config.Load(runtimeConfigPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	gatewayConfig, err := loader.Load(gatewayConfigPath)
	if err != nil {
		t.Fatalf("loader.Load: %v", err)
	}

	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	deps, err := app.Bootstrap(runtimeConfig, gatewayConfig)
	if err != nil {
		t.Fatalf("app.Bootstrap: %v", err)
	}

	return deps
}

func TestGateway_ProxiesMatchedRouteAndAppliesHeaderPolicy(t *testing.T) {
	upstream, _ := newMockUpstream(t)
	deps := bootstrapTestGateway(t, upstream)

	gw := httptest.NewServer(deps.PublicHTTP.Handler)
	t.Cleanup(gw.Close)

	req, err := http.NewRequest(http.MethodGet, gw.URL+"/api/v1/profile", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("X-Forwarded-For", "203.0.113.1") // must be stripped by the policy before reaching upstream

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", resp.StatusCode, http.StatusOK, body)
	}
	if got := string(body); got != `{"status":"ok"}` {
		t.Errorf("body = %q, want %q", got, `{"status":"ok"}`)
	}

	// Response-side policy: headers added by security-headers must be present.
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want %q", got, "DENY")
	}
}

// TestGateway_RequestSidePolicyReachesUpstream verifies request-side header
// mutations by inspecting the headers the mock upstream actually received —
// not by inferring them from the gateway's response, which only proves the
// response-side policy ran.
func TestGateway_RequestSidePolicyReachesUpstream(t *testing.T) {
	upstream, captured := newMockUpstream(t)
	deps := bootstrapTestGateway(t, upstream)

	gw := httptest.NewServer(deps.PublicHTTP.Handler)
	t.Cleanup(gw.Close)

	req, err := http.NewRequest(http.MethodGet, gw.URL+"/api/v1/profile", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("X-Forwarded-For", "203.0.113.1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if got := captured.Get("X-Forwarded-For"); got != "" {
		t.Errorf("X-Forwarded-For reached upstream as %q, want stripped by policy", got)
	}
	if got := captured.Get("X-Request-Id"); got != "e2e-test" {
		t.Errorf("X-Request-Id reached upstream as %q, want %q", got, "e2e-test")
	}
}

func TestGateway_MethodNotAllowed(t *testing.T) {
	upstream, _ := newMockUpstream(t)
	deps := bootstrapTestGateway(t, upstream)

	gw := httptest.NewServer(deps.PublicHTTP.Handler)
	t.Cleanup(gw.Close)

	resp, err := http.Post(gw.URL+"/api/v1/profile", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestGateway_UnknownRouteReturnsNotFound(t *testing.T) {
	upstream, _ := newMockUpstream(t)
	deps := bootstrapTestGateway(t, upstream)

	gw := httptest.NewServer(deps.PublicHTTP.Handler)
	t.Cleanup(gw.Close)

	resp, err := http.Get(gw.URL + "/does/not/exist")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestGateway_SystemListenerReportsLiveness(t *testing.T) {
	upstream, _ := newMockUpstream(t)
	deps := bootstrapTestGateway(t, upstream)

	sys := httptest.NewServer(deps.SystemHTTP.Handler)
	t.Cleanup(sys.Close)

	resp, err := http.Get(sys.URL + "/livez")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
