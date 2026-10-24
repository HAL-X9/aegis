//go:build !race

package proxy

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

// This file is excluded under the race detector: with -race, sync.Pool
// deliberately drops a share of the objects it is given, so the buffer pool on
// the response path makes the allocation count nondeterministic. The plain
// `go test ./...` run enforces the budgets; `go test ./... -race` skips them.

// TestServeHTTPAllocationBudget pins the number of heap allocations of one
// proxied request, so that a refactoring of ServeHTTP cannot add allocations
// without a test failing. The same paths are measured, with timing, by the
// BenchmarkProxyServeHTTP family.
//
// Each budget is the current count, the floor set by net/http (the outbound
// request and its header map, and the headers copied from the upstream
// response). Raise a budget only with a reason recorded in the commit message.
func TestServeHTTPAllocationBudget(t *testing.T) {
	tests := map[string]struct {
		policy  bool
		request func() *http.Request
		budget  float64
	}{
		"typical headers": {
			request: buildProxyBenchRequest,
			budget:  4,
		},
		"minimal request": {
			request: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "http://gateway.local/api/v1/profile", nil)
			},
			budget: 4,
		},
		"header policies": {
			policy: true,
			request: func() *http.Request {
				req := buildProxyBenchRequest()
				req.Header.Set("X-Forwarded-For", "203.0.113.1")
				return req
			},
			budget: 4,
		},
		"twenty extra request headers": {
			request: func() *http.Request {
				req := buildProxyBenchRequest()
				for i := range 20 {
					req.Header.Set("X-Benchmark-"+strconv.Itoa(i), "value-"+strconv.Itoa(i))
				}
				return req
			},
			budget: 6,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			engine := buildProxyBenchEngine(t)
			if tc.policy {
				engine = buildProxyBenchEngineWithPolicy(t)
			}
			transport := newBenchmarkTransport(
				http.Header{"Content-Type": {"text/plain"}},
				[]byte("hello"),
			)
			executor := newBenchExecutor(t, engine, transport)

			req := tc.request()
			w := newBenchmarkWriter()

			got := testing.AllocsPerRun(200, func() { executor.ServeHTTP(w, req) })

			switch {
			case got > tc.budget:
				t.Errorf("ServeHTTP allocated %v times per request, budget is %v", got, tc.budget)
			case got < tc.budget:
				t.Logf("ServeHTTP allocated %v times per request, below the budget of %v: lower the budget", got, tc.budget)
			}
		})
	}
}
