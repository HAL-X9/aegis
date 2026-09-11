package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
)

// This file answers one specific question that a sequential benchmark cannot:
// does throughput scale with concurrency, or does it plateau early?
//
// A sequential RoundTrip benchmark (BenchmarkRoundTripWarm in
// executor_network_bench_test.go) reports wall-clock latency per call. Most
// of that latency is the goroutine parked in a read() syscall waiting for
// the kernel/upstream — time during which the OS thread is free to run other
// goroutines. Sequential ns/op therefore systematically understates
// achievable throughput for I/O-bound work.
//
// These benchmarks instead measure throughput under realistic concurrency:
// many goroutines issuing RoundTrip calls against the *same* shared
// http.Transport at once, exactly as production traffic would. Falling
// ns/op as parallelism increases means the work was I/O-wait, and CPU is
// not the bottleneck. A plateau or rising ns/op at higher parallelism means
// something inside Transport (connection-pool bookkeeping, a shared mutex,
// GC pressure from the 43 allocs/call) is serializing work that should be
// parallel.
//
// Run (sweep both OS-thread parallelism and logical goroutine multiplier):
//
//	go test ./internal/dataplane/proxy/ -run '^$' \
//	    -bench 'RoundTripParallel|FullProxyE2EParallel' \
//	    -benchmem -cpu 1,2,4,8,16 -benchtime 2s -count 5
//
// To see *why* (CPU-bound vs blocked-in-syscall vs GC), capture an
// execution trace on one high-parallelism run and inspect it visually:
//
//	go test ./internal/dataplane/proxy/ -run '^$' \
//	    -bench 'BenchmarkRoundTripParallel/mult=64' -cpu 8 \
//	    -trace trace.out
//	go tool trace trace.out
//
// In the trace UI, look at the per-proc timeline: mostly "syscall"/"waiting"
// bands confirm I/O-wait; mostly "running" bands with few procs busy while
// others idle points at contention, not raw RoundTrip cost.
//
// To find a specific lock if the trace shows contention:
//
//	go test ./internal/dataplane/proxy/ -run '^$' \
//	    -bench 'BenchmarkRoundTripParallel/mult=64' -cpu 8 \
//	    -mutexprofile mutex.out -blockprofile block.out
//	go tool pprof -top block.out
//	go tool pprof -top mutex.out

// parallelismMultipliers control b.SetParallelism: the total number of
// concurrent goroutines per benchmark is GOMAXPROCS * multiplier. Sweeping
// this independently of -cpu is what actually distinguishes "CPU-bound"
// from "I/O-bound, needs more in-flight requests than cores to saturate":
// I/O-bound work keeps improving well past multiplier=1 because goroutines
// blocked in syscalls don't occupy a core.
var parallelismMultipliers = []int{1, 4, 16, 64}

// warmConnections establishes n concurrent keep-alive connections to target
// before a benchmark's timer starts. A single warm-up request (as used in
// the sequential BenchmarkRoundTripWarm) only opens one connection; at
// higher parallelism the transport would otherwise spend the first wave of
// "measured" iterations paying dial+handshake cost for new connections,
// contaminating the result with cold-start noise instead of steady-state
// throughput.
func warmConnections(tb testing.TB, transport *http.Transport, url string, n int) {
	tb.Helper()

	var wg sync.WaitGroup
	errs := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
			if err != nil {
				errs <- err
				return
			}
			resp, err := transport.RoundTrip(req)
			if err != nil {
				errs <- err
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			tb.Fatalf("warm-up RoundTrip failed: %v", err)
		}
	}
}

// BenchmarkRoundTripParallel measures throughput of http.Transport.RoundTrip
// alone (no Executor, no routing) under concurrent load against a local
// upstream, at several concurrency levels. This isolates whether Transport
// itself scales with parallelism or serializes internally.
func BenchmarkRoundTripParallel(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	for _, mult := range parallelismMultipliers {
		mult := mult
		b.Run(concatMult(mult), func(b *testing.B) {
			transport := &http.Transport{
				MaxIdleConnsPerHost: 4096,
				MaxConnsPerHost:     0, // unbounded: don't let the pool cap be the bottleneck under test
				DisableCompression:  true,
			}
			defer transport.CloseIdleConnections()

			warmConnections(b, transport, upstream.URL, mult*8)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, upstream.URL, nil)
			if err != nil {
				b.Fatal(err)
			}

			b.SetParallelism(mult)
			b.ReportAllocs()
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					resp, err := transport.RoundTrip(req)
					if err != nil {
						b.Fatal(err)
					}
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}
			})
		})
	}
}

// buildParallelE2EEngine mirrors buildE2EBenchEngine (executor_network_bench_test.go).
func buildParallelE2EEngine(tb testing.TB, upstreamURL string) *router.Engine {
	tb.Helper()

	cfg := &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{
			Items: []snapshot.CompiledService{{Name: "profile", Upstream: upstreamURL}},
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

// BenchmarkFullProxyE2EParallel measures the complete path (client -> gateway
// -> Executor -> upstream -> back) under concurrent load, at the same
// parallelism levels as BenchmarkRoundTripParallel, so the two can be
// compared directly to see how much of end-to-end throughput is explained
// by transport behavior alone vs. Executor/gateway-listener overhead.
func BenchmarkFullProxyE2EParallel(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	engine := buildParallelE2EEngine(b, upstream.URL)

	for _, mult := range parallelismMultipliers {
		mult := mult
		b.Run(concatMult(mult), func(b *testing.B) {
			serverTransport := &http.Transport{
				MaxIdleConnsPerHost: 4096,
				MaxConnsPerHost:     0,
				DisableCompression:  true,
			}
			defer serverTransport.CloseIdleConnections()

			executor := NewExecutor(engine, noRateLimits(), serverTransport)
			gw := httptest.NewServer(executor)
			defer gw.Close()

			clientTransport := &http.Transport{
				MaxIdleConnsPerHost: 4096,
				MaxConnsPerHost:     0,
			}
			defer clientTransport.CloseIdleConnections()
			client := &http.Client{Transport: clientTransport}

			warmConnections(b, clientTransport, gw.URL+"/api/v1/profile", mult*8)

			b.SetParallelism(mult)
			b.ReportAllocs()
			b.ResetTimer()

			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
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
			})
		})
	}
}

func concatMult(mult int) string {
	switch mult {
	case 1:
		return "mult=1"
	case 4:
		return "mult=4"
	case 16:
		return "mult=16"
	case 64:
		return "mult=64"
	default:
		return "mult=other"
	}
}
