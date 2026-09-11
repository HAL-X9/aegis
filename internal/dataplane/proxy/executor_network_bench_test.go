package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
)

// This file isolates network/transport cost from the pure Executor cost
// measured in executor_bench_test.go (BenchmarkProxyServeHTTP, which uses
// benchmarkTransport and therefore performs zero real I/O).
//
// Together the three layers answer one question: how much of total request
// latency is Aegis's own code vs. net/http.Transport/the network itself.
//
//   BenchmarkProxyServeHTTP   (existing, executor_bench_test.go) — Aegis only
//   BenchmarkRoundTripWarm    (this file)                        — transport only
//   BenchmarkFullProxyE2E     (this file)                        — everything
//
// Run all three together for comparison:
//
//	go test ./internal/dataplane/proxy/ -run '^$' \
//	    -bench 'BenchmarkProxyServeHTTP$|BenchmarkRoundTripWarm|BenchmarkFullProxyE2E' \
//	    -benchmem -count 10

// BenchmarkRoundTripWarm measures only http.Transport.RoundTrip against a
// real local upstream over an already-warm keep-alive connection. No
// Executor, no routing, no header mutation — this is the stdlib transport
// floor cost that Aegis-owned code cannot reduce without replacing Transport.
func BenchmarkRoundTripWarm(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	transport := &http.Transport{
		MaxIdleConnsPerHost: 100,
		DisableCompression:  true,
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, upstream.URL, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Warm the connection before measuring — otherwise the first iteration
	// pays TCP handshake and skews the result.
	warm, err := transport.RoundTrip(req)
	if err != nil {
		b.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, warm.Body)
	_ = warm.Body.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp, err := transport.RoundTrip(req)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

// buildE2EBenchEngine mirrors buildProxyBenchEngine but points the compiled
// upstream at a real local httptest.Server instead of a fake host, so the
// request can actually be proxied end-to-end.
func buildE2EBenchEngine(tb testing.TB, upstreamURL string) *router.Engine {
	tb.Helper()

	cfg := &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{
			Items: []snapshot.CompiledService{
				{Name: "profile", Upstream: upstreamURL},
			},
		},
		Routes: []snapshot.CompiledRoute{
			{
				Name:    "profile",
				Service: snapshot.ServiceID(0),
				Match: snapshot.CompiledMatch{
					PathPrefix: "/api/v1/profile",
					Methods:    methodmask.MethodAll,
				},
			},
		},
	}

	engine, err := router.BuildEngine(cfg)
	if err != nil {
		tb.Fatalf("BuildEngine: %v", err)
	}
	return engine
}

// BenchmarkFullProxyE2E measures the complete path: real client -> real
// gateway listener -> Executor -> real transport -> real upstream -> back.
// This is the number that should match production order of magnitude; the
// other two benchmarks exist to explain *why* it looks the way it does.
func BenchmarkFullProxyE2E(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	engine := buildE2EBenchEngine(b, upstream.URL)
	transport := &http.Transport{MaxIdleConnsPerHost: 100, DisableCompression: true}
	executor := NewExecutor(engine, noRateLimits(), transport)

	gw := httptest.NewServer(executor)
	defer gw.Close()

	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 100}}

	warmReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, gw.URL+"/api/v1/profile", nil)
	if err != nil {
		b.Fatal(err)
	}
	if resp, err := client.Do(warmReq); err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	} else {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, gw.URL+"/api/v1/profile", nil)
		if err != nil {
			b.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
