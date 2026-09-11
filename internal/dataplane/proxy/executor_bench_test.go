package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
)

// These benchmarks measure Executor.ServeHTTP's actual allocation profile:
// route lookup, method/header admission, the pooled upstream request/URL,
// header-policy execution, and response streaming — using a fixed,
// allocation-free RoundTripper so numbers reported here are the proxy's
// own cost, not real network I/O.
//
// Run:
//
//	go test ./internal/dataplane/proxy/ -run '^$' \
//	    -bench 'ProxyServeHTTP' \
//	    -benchmem -count 10
//
// For before/after comparison:
//
//	go test ./internal/dataplane/proxy/ -run '^$' \
//	    -bench 'ProxyServeHTTP' \
//	    -benchmem -count 10 > before.txt
//
//	benchstat before.txt after.txt
//
// To find the source of any residual allocation:
//
//	go test ./internal/dataplane/proxy/ -run '^$' \
//	    -bench 'BenchmarkProxyServeHTTP$' -benchmem \
//	    -memprofile mem.out -count 1
//	go tool pprof -alloc_objects -list='ServeHTTP' mem.out

// benchmarkResponseWriter intentionally implements only the ResponseWriter
// interface. It does not perform any actual network I/O. header is
// allocated fresh per iteration outside the timed region in each benchmark
// below, matching what a real net/http.Server does per incoming request
// (server.go's response type always starts with a fresh Header map) — this
// is deliberately NOT hidden or pooled away, since it isn't Aegis's own
// allocation to eliminate.
type benchmarkResponseWriter struct {
	header     http.Header
	statusCode int
}

func (w *benchmarkResponseWriter) Header() http.Header {
	return w.header
}

func (w *benchmarkResponseWriter) WriteHeader(code int) {
	w.statusCode = code
}

func (w *benchmarkResponseWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

// benchmarkBody is a tiny, reusable response body. pos is reset before each
// RoundTrip so the same instance can be reused without reallocating —
// this keeps benchmarkTransport itself allocation-free, so any allocs
// reported by the benchmark are attributable to Executor, not the harness.
type benchmarkBody struct {
	data []byte
	pos  int
}

func (b *benchmarkBody) Read(p []byte) (int, error) {
	if b.pos >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}

func (b *benchmarkBody) Close() error {
	return nil
}

// benchmarkTransport avoids network activity completely.
//
// This is important. The purpose of this benchmark is to measure the proxy
// request-processing path, not Linux sockets, Docker networking, or
// upstream latency — those are covered separately in
// executor_network_bench_test.go and executor_parallel_bench_test.go.
type benchmarkTransport struct {
	responseHeader http.Header
	responseBody   []byte
}

func (t *benchmarkTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     t.responseHeader,
		Body: &benchmarkBody{
			data: t.responseBody,
		},
		Request: req,
	}, nil
}

// buildProxyBenchEngine creates exactly one route:
//
//	/api/v1/profile
//
// The route has no header predicates and accepts all methods, matching a
// bare hot-path measurement with no policy overhead. See
// buildProxyBenchEngineWithPolicy below for the policy-bearing variant.
func buildProxyBenchEngine(tb testing.TB) *router.Engine {
	tb.Helper()

	cfg := &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{
			Items: []snapshot.CompiledService{
				{
					Name:     "profile",
					Upstream: "http://upstream.internal:8080",
				},
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

// buildProxyBenchEngineWithPolicy mirrors buildProxyBenchEngine but attaches
// a request+response header policy to the route, so ExecuteMutations'
// actual cost on the hot path is visible instead of hidden by an
// empty-policy benchmark. Values mirror configs/gateway.yaml's
// security-headers policy.
func buildProxyBenchEngineWithPolicy(tb testing.TB) *router.Engine {
	tb.Helper()

	cfg := &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{
			Items: []snapshot.CompiledService{
				{
					Name:     "profile",
					Upstream: "http://upstream.internal:8080",
				},
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
				Policies: snapshot.CompiledRoutePolicies{
					RateLimitID: -1,
					Headers: snapshot.CompiledHeaders{
						Request: snapshot.CompiledHeadersPlan{
							Ops: []snapshot.HeaderInstruction{
								{
									HeaderID: snapshot.HeaderXRequestID,
									Op:       snapshot.HeaderOpAddIfAbsent,
									Value:    "generated",
								},
								{
									HeaderID: snapshot.HeaderXForwardedFor,
									Op:       snapshot.HeaderOpRemove,
								},
							},
						},
						Response: snapshot.CompiledHeadersPlan{
							Ops: []snapshot.HeaderInstruction{
								{
									HeaderID: snapshot.HeaderXContentTypeOptions,
									Op:       snapshot.HeaderOpAddIfAbsent,
									Value:    "nosniff",
								},
								{
									HeaderID: snapshot.HeaderXFrameOptions,
									Op:       snapshot.HeaderOpAddIfAbsent,
									Value:    "DENY",
								},
								{
									HeaderID: snapshot.HeaderServer,
									Op:       snapshot.HeaderOpRemove,
								},
							},
						},
					},
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

func buildProxyBenchRequest() *http.Request {
	req := httptest.NewRequest(
		http.MethodGet,
		"http://gateway.local/api/v1/profile",
		nil,
	)

	// These are intentionally representative normal HTTP headers.
	req.Header.Set("Host", "gateway.local")
	req.Header.Set("User-Agent", "benchmark")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("X-Request-ID", "benchmark-request")
	req.Header.Set("X-Tenant", "tenant-1")

	return req
}

// BenchmarkProxyServeHTTP measures the complete Executor hot path with a
// bare route (no header policy, no rate limiting) while completely
// removing real network I/O. This is the benchmark to use when evaluating
// changes to ServeHTTP itself, and the one to compare against pprof
// profiles taken with -memprofile.
func BenchmarkProxyServeHTTP(b *testing.B) {
	engine := buildProxyBenchEngine(b)

	transport := &benchmarkTransport{
		responseHeader: http.Header{
			"Content-Type": {"text/plain"},
			"X-Upstream":   {"benchmark"},
		},
		responseBody: []byte("hello"),
	}

	executor := NewExecutor(engine, noRateLimits(), transport)

	req := buildProxyBenchRequest()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := &benchmarkResponseWriter{
			header: make(http.Header),
		}

		executor.ServeHTTP(w, req)
	}
}

// BenchmarkProxyServeHTTP_EmptyHeaders isolates the common case where the
// incoming request contains almost no headers, to see how much of total
// cost scales with header count vs. is fixed per-request overhead.
func BenchmarkProxyServeHTTP_EmptyHeaders(b *testing.B) {
	engine := buildProxyBenchEngine(b)

	transport := &benchmarkTransport{
		responseHeader: http.Header{},
		responseBody:   []byte("hello"),
	}

	executor := NewExecutor(engine, noRateLimits(), transport)

	req := httptest.NewRequest(
		http.MethodGet,
		"http://gateway.local/api/v1/profile",
		nil,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := &benchmarkResponseWriter{
			header: make(http.Header),
		}

		executor.ServeHTTP(w, req)
	}
}

// BenchmarkProxyServeHTTP_RealisticHeaders uses several normal HTTP
// headers to make header-count scaling visible.
func BenchmarkProxyServeHTTP_RealisticHeaders(b *testing.B) {
	engine := buildProxyBenchEngine(b)

	transport := &benchmarkTransport{
		responseHeader: http.Header{},
		responseBody:   []byte("hello"),
	}

	executor := NewExecutor(engine, noRateLimits(), transport)

	req := buildProxyBenchRequest()

	// Add a moderately realistic header set.
	for i := 0; i < 20; i++ {
		req.Header.Set(
			"X-Benchmark-"+strconv.Itoa(i),
			"value-"+strconv.Itoa(i),
		)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := &benchmarkResponseWriter{
			header: make(http.Header),
		}

		executor.ServeHTTP(w, req)
	}
}

// BenchmarkProxyServeHTTP_WithHeaderPolicy measures the hot path with a
// real request+response header policy attached to the route (mirroring
// configs/gateway.yaml's security-headers), so ExecuteMutations' cost is
// part of the measured number instead of hidden by every other benchmark
// in this file using a policy-free route.
func BenchmarkProxyServeHTTP_WithHeaderPolicy(b *testing.B) {
	engine := buildProxyBenchEngineWithPolicy(b)

	transport := &benchmarkTransport{
		responseHeader: http.Header{
			"Content-Type": {"text/plain"},
		},
		responseBody: []byte("hello"),
	}

	executor := NewExecutor(engine, noRateLimits(), transport)

	req := buildProxyBenchRequest()
	req.Header.Set("X-Forwarded-For", "203.0.113.1") // exercised by the remove op

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := &benchmarkResponseWriter{
			header: make(http.Header),
		}

		executor.ServeHTTP(w, req)
	}
}

// BenchmarkProxyServeHTTPParallel measures ServeHTTP under concurrent load
// against the shared upstreamRequestPool. This is the benchmark that would
// surface pool contention (lock/CAS overhead, or a shrinking hit rate as
// goroutines outnumber pooled objects) that a sequential benchmark cannot:
// every goroutine here Gets/Puts the same package-level sync.Pool at once,
// exactly as concurrent real requests would.
func BenchmarkProxyServeHTTPParallel(b *testing.B) {
	engine := buildProxyBenchEngine(b)

	transport := &benchmarkTransport{
		responseHeader: http.Header{
			"Content-Type": {"text/plain"},
		},
		responseBody: []byte("hello"),
	}

	executor := NewExecutor(engine, noRateLimits(), transport)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine uses its own *http.Request: sharing one across
		// goroutines would race on combo.req = *r reading it concurrently
		// with nothing writing it — that's not a race in practice since r
		// is never mutated, but giving each goroutine its own instance
		// avoids relying on that and keeps this benchmark exactly matching
		// how a real server dispatches one *http.Request per goroutine.
		req := buildProxyBenchRequest()

		for pb.Next() {
			w := &benchmarkResponseWriter{
				header: make(http.Header),
			}

			executor.ServeHTTP(w, req)
		}
	})
}
