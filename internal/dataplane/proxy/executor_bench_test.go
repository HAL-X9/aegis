package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

// These benchmarks measure Executor.ServeHTTP without real network I/O.
//
// The benchmark transport is deliberately deterministic and keeps the
// upstream response state local to each benchmark invocation. This lets us
// measure Aegis request processing without TCP, TLS, kernel scheduling,
// or real upstream latency.
//
// Run:
//
//	go test ./internal/dataplane/proxy/ \
//	    -run '^$' \
//	    -bench 'ProxyServeHTTP' \
//	    -benchmem \
//	    -count 10
//
// For a lower-noise before/after comparison:
//
//	go test ./internal/dataplane/proxy/ \
//	    -run '^$' \
//	    -bench 'ProxyServeHTTP' \
//	    -benchmem \
//	    -benchtime 3s \
//	    -count 10
//
// CPU profile:
//
//	go test ./internal/dataplane/proxy/ \
//	    -run '^$' \
//	    -bench '^BenchmarkProxyServeHTTP$' \
//	    -benchtime 10s \
//	    -cpuprofile cpu.out \
//	    -count 1
//
// Allocation profile:
//
//	go test ./internal/dataplane/proxy/ \
//	    -run '^$' \
//	    -bench '^BenchmarkProxyServeHTTP$' \
//	    -benchtime 10s \
//	    -memprofile mem.out \
//	    -count 1
//
// Then:
//
//	go tool pprof -top cpu.out
//	go tool pprof -alloc_objects -top mem.out
//	go tool pprof -alloc_space -top mem.out

// benchmarkResponseWriter implements only http.ResponseWriter.
// It performs no network I/O.
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

// benchmarkBody is reusable response-body state.
//
// A body belongs to one benchmarkTransport invocation. It is reset before
// every RoundTrip and therefore does not need to be allocated by the
// transport.
type benchmarkBody struct {
	data []byte
	pos  int
}

func (b *benchmarkBody) reset(data []byte) {
	b.data = data
	b.pos = 0
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

// benchmarkResponse is reusable http.Response state.
type benchmarkResponse struct {
	response http.Response
	body     benchmarkBody
}

// benchmarkTransport performs no network I/O.
//
// All state belongs to the transport instance. Each benchmark goroutine gets
// its own transport, so response/header maps are never shared between
// concurrent benchmark invocations.
type benchmarkTransport struct {
	responseHeader http.Header
	responseBody   []byte
	response       benchmarkResponse
}

func newBenchmarkTransport(
	responseHeader http.Header,
	responseBody []byte,
) *benchmarkTransport {
	return &benchmarkTransport{
		responseHeader: responseHeader,
		responseBody:   responseBody,
		response: benchmarkResponse{
			response: http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     responseHeader,
			},
		},
	}
}

func (t *benchmarkTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.response.body.reset(t.responseBody)
	t.response.response.Body = &t.response.body
	t.response.response.Request = req

	return &t.response.response, nil
}

func buildProxyBenchEngine(tb testing.TB) *router.Engine {
	tb.Helper()

	cfg := &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{
			Items: []snapshot.CompiledService{
				{
					Name:     "profile",
					Upstream: testURL("http://upstream.internal:8080"),
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

	engine, err := newTestEngine(cfg)
	if err != nil {
		tb.Fatalf("newTestEngine: %v", err)
	}

	return engine
}

func buildProxyBenchEngineWithPolicy(tb testing.TB) *router.Engine {
	tb.Helper()

	cfg := &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{
			Items: []snapshot.CompiledService{
				{
					Name:     "profile",
					Upstream: testURL("http://upstream.internal:8080"),
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

	engine, err := newTestEngine(cfg)
	if err != nil {
		tb.Fatalf("newTestEngine: %v", err)
	}

	return engine
}

// buildProxyBenchRequest returns a request with a typical header set.
// ServeHTTP does not modify its request, so one request can be served
// repeatedly.
func buildProxyBenchRequest() *http.Request {
	req := httptest.NewRequest(
		http.MethodGet,
		"http://gateway.local/api/v1/profile",
		nil,
	)

	req.Header.Set("Host", "gateway.local")
	req.Header.Set("User-Agent", "benchmark")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("X-Request-ID", "benchmark-request")
	req.Header.Set("X-Tenant", "tenant-1")

	return req
}

func newBenchmarkWriter() *benchmarkResponseWriter {
	return &benchmarkResponseWriter{
		header: make(http.Header),
	}
}

// BenchmarkProxyServeHTTP measures the Executor hot path with:
//
//   - compiled route lookup
//   - method admission
//   - upstream request preparation
//   - fake RoundTrip
//   - response handling
//   - response body streaming
//
// There is no real network I/O.
func BenchmarkProxyServeHTTP(b *testing.B) {
	engine := buildProxyBenchEngine(b)

	transport := newBenchmarkTransport(
		http.Header{
			"Content-Type": {"text/plain"},
			"X-Upstream":   {"benchmark"},
		},
		[]byte("hello"),
	)

	executor := newBenchExecutor(b, engine, transport)

	req := buildProxyBenchRequest()
	w := newBenchmarkWriter()

	b.ReportAllocs()
	for b.Loop() {
		executor.ServeHTTP(w, req)
	}
}

// BenchmarkProxyServeHTTP_EmptyHeaders measures the common case with a
// minimal incoming request.
func BenchmarkProxyServeHTTP_EmptyHeaders(b *testing.B) {
	engine := buildProxyBenchEngine(b)

	transport := newBenchmarkTransport(
		http.Header{},
		[]byte("hello"),
	)

	executor := newBenchExecutor(b, engine, transport)

	req := httptest.NewRequest(http.MethodGet, "http://gateway.local/api/v1/profile", nil)
	w := newBenchmarkWriter()

	b.ReportAllocs()
	for b.Loop() {
		executor.ServeHTTP(w, req)
	}
}

// BenchmarkProxyServeHTTP_RealisticHeaders measures request processing with
// a moderately realistic HTTP header set.
func BenchmarkProxyServeHTTP_RealisticHeaders(b *testing.B) {
	engine := buildProxyBenchEngine(b)

	transport := newBenchmarkTransport(
		http.Header{},
		[]byte("hello"),
	)

	executor := newBenchExecutor(b, engine, transport)

	req := buildProxyBenchRequest()
	for j := range 20 {
		req.Header.Set("X-Benchmark-"+strconv.Itoa(j), "value-"+strconv.Itoa(j))
	}
	w := newBenchmarkWriter()

	b.ReportAllocs()
	for b.Loop() {
		executor.ServeHTTP(w, req)
	}
}

// BenchmarkProxyServeHTTP_WithHeaderPolicy measures the same path with
// request and response header policies enabled.
func BenchmarkProxyServeHTTP_WithHeaderPolicy(b *testing.B) {
	engine := buildProxyBenchEngineWithPolicy(b)

	transport := newBenchmarkTransport(
		http.Header{
			"Content-Type": {"text/plain"},
		},
		[]byte("hello"),
	)

	executor := newBenchExecutor(b, engine, transport)

	req := buildProxyBenchRequest()
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	w := newBenchmarkWriter()

	b.ReportAllocs()
	for b.Loop() {
		executor.ServeHTTP(w, req)
	}
}

// BenchmarkProxyServeHTTPParallel measures concurrent Executor execution.
// Each worker owns its transport and response writer, whose header maps
// are written by ServeHTTP, and its request.
func BenchmarkProxyServeHTTPParallel(b *testing.B) {
	engine := buildProxyBenchEngine(b)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		transport := newBenchmarkTransport(
			http.Header{
				"Content-Type": {"text/plain"},
			},
			[]byte("hello"),
		)

		executor := newBenchExecutor(b, engine, transport)
		req := buildProxyBenchRequest()
		w := newBenchmarkWriter()

		for pb.Next() {
			executor.ServeHTTP(w, req)
		}
	})
}

// BenchmarkProxyServeHTTP_RealTransport measures ServeHTTP with a real
// *http.Transport against a local upstream, so the result includes
// transport and loopback network cost.
func BenchmarkProxyServeHTTP_RealTransport(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Upstream", "benchmark")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer upstream.Close()

	engine := buildProxyBenchEngineWithUpstream(b, upstream.URL)

	transport := &http.Transport{}
	defer transport.CloseIdleConnections()

	executor := newBenchExecutor(b, engine, transport)
	req := buildProxyBenchRequest()
	w := newBenchmarkWriter()

	b.ReportAllocs()
	for b.Loop() {
		executor.ServeHTTP(w, req)
	}
}

func buildProxyBenchEngineWithUpstream(
	tb testing.TB,
	upstreamURL string,
) *router.Engine {
	tb.Helper()

	cfg := &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{
			Items: []snapshot.CompiledService{
				{
					Name:     "profile",
					Upstream: testURL(upstreamURL),
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

	engine, err := newTestEngine(cfg)
	if err != nil {
		tb.Fatalf("newTestEngine: %v", err)
	}

	return engine
}
