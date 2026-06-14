package router

// Enterprise request hot-path benchmarks for the aegis data plane.
//
// These benchmarks measure the cost of the per-request matching pipeline that
// every proxied request pays in production:
//
//  1. radix path lookup            (Engine.Lookup)
//  2. HTTP method admission        (methodmask bitwise check)
//  3. header predicate evaluation  (HeadersMatch)
//
// The suite is structured as an orthogonal matrix so the marginal cost of each
// stage can be attributed independently:
//
//	methods  ∈ { unrestricted (MethodAll), restricted (GET|POST) }
//	headers  ∈ { none, presence-only, value-constrained }
//
// Every benchmark calls b.ReportAllocs(); the matching path is expected to be
// allocation-free (0 allocs/op). A non-zero alloc count is a regression and
// should fail review even if ns/op looks acceptable.
//
// Run:
//
//	go test ./internal/dataplane/router/ -run '^$' -bench 'Request|Lookup|Headers|Method' -benchmem
//	go test ./internal/dataplane/router/ -run '^$' -bench Request -benchmem -benchtime 3s -count 10 > new.txt
//	benchstat old.txt new.txt
//
// Pin the measurement environment for comparable numbers:
//
//	GOMAXPROCS=1 GOGC=off go test ... -benchmem -count 10

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/aegis/internal/contracts/methodmask"
	"github.com/aegis/internal/controlplane/snapshot"
)

// sink prevents the compiler from eliminating benchmark work via dead-code
// elimination. Results are accumulated here and never read.
var (
	sinkEntries []*RouteIndexEntry
	sinkBool    bool
)

// routingTableSize is the number of compiled routes loaded into the engine for
// hot-path benchmarks. It is sized to resemble a non-trivial production tenant
// rather than a toy table, so trie traversal and child fan-out are exercised.
const routingTableSize = 256

// headerMode selects which header predicate set a route is compiled with.
type headerMode int

const (
	headerNone     headerMode = iota // no header predicates (fast path)
	headerPresence                   // header must be present, any value
	headerValue                      // header must equal one of N allowed values
)

// methodMode selects the route's method admission policy.
type methodMode int

const (
	methodUnrestricted methodMode = iota // MethodAll: no method filtering
	methodRestricted                     // explicit GET|POST allow-list
)

// benchHeaderName is the canonical header key used by header-bearing routes.
// HeadersMatch indexes http.Header directly by the compiled (canonical) key,
// so request headers below are written with the same canonical form.
var benchHeaderName = http.CanonicalHeaderKey("X-Tenant-Tier")

// buildBenchEngine compiles a routing table of the requested size and returns
// the engine together with a representative request path that resolves to a
// terminal route. Routes are deep (4 segments) and statically fanned out to
// keep the radix traversal honest.
func buildBenchEngine(tb testing.TB, size int, mm methodMode, hm headerMode) (*Engine, []byte) {
	tb.Helper()

	var mask methodmask.MethodMask
	switch mm {
	case methodUnrestricted:
		mask = methodmask.MethodAll
	case methodRestricted:
		mask = methodmask.MethodGET | methodmask.MethodPOST
	}

	var preds []snapshot.HeaderPredicate
	switch hm {
	case headerPresence:
		preds = []snapshot.HeaderPredicate{{Name: benchHeaderName}}
	case headerValue:
		preds = []snapshot.HeaderPredicate{{
			Name:          benchHeaderName,
			AllowedValues: []string{"bronze", "silver", "gold", "platinum"},
		}}
	}

	routes := make([]snapshot.CompiledRoute, 0, size)
	for i := 0; i < size; i++ {
		id := strconv.Itoa(i)
		routes = append(routes, snapshot.CompiledRoute{
			Name: "svc-" + id,
			Match: snapshot.CompiledMatch{
				PathPrefix: "/api/v1/service-" + id + "/resource",
				Methods:    mask,
				Headers:    preds,
			},
			Upstream: "http://upstream-" + id + ".internal:8080",
		})
	}

	engine, err := BuildEngine(&snapshot.CompiledConfig{Routes: routes})
	if err != nil {
		tb.Fatalf("BuildEngine: %v", err)
	}

	// Resolve a route in the middle of the table to avoid best/worst-case bias
	// from edge ordering of sibling children.
	target := strconv.Itoa(size / 2)
	path := []byte("/api/v1/service-" + target + "/resource")

	// Sanity check the fixture so a broken setup fails loudly rather than
	// silently benchmarking a no-match path.
	if got := engine.Lookup(path); len(got) == 0 {
		tb.Fatalf("fixture path %q did not resolve to a route", path)
	}

	return engine, path
}

// requestMethodBit resolves an incoming request method to its mask bit. This is
// the realistic per-request cost paid on the method-admission path; it is kept
// inside the measured loop deliberately.
func requestMethodBit(method string) methodmask.MethodMask {
	bit, _ := methodmask.MethodBit(method)
	return bit
}

// BenchmarkLookup measures the radix path lookup in isolation across table
// sizes. This is the floor cost every request pays regardless of policy.
func BenchmarkLookup(b *testing.B) {
	for _, size := range []int{16, 64, 256, 1024} {
		engine, path := buildBenchEngine(b, size, methodUnrestricted, headerNone)
		b.Run("routes="+strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			var out []*RouteIndexEntry
			for i := 0; i < b.N; i++ {
				out = engine.Lookup(path)
			}
			sinkEntries = out
		})
	}
}

// BenchmarkMethodAdmission measures only the method resolution + bitmask check,
// contrasting an unrestricted route (MethodAll) against a restricted one.
func BenchmarkMethodAdmission(b *testing.B) {
	cases := []struct {
		name string
		mode methodMode
	}{
		{"unrestricted", methodUnrestricted},
		{"restricted", methodRestricted},
	}
	for _, tc := range cases {
		engine, path := buildBenchEngine(b, routingTableSize, tc.mode, headerNone)
		entry := engine.Lookup(path)[0]
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			var ok bool
			for i := 0; i < b.N; i++ {
				ok = entry.Route.Match.Methods&requestMethodBit(http.MethodGet) != 0
			}
			sinkBool = ok
		})
	}
}

// BenchmarkHeadersMatch measures header predicate evaluation in isolation for
// the three supported header modes.
func BenchmarkHeadersMatch(b *testing.B) {
	cases := []struct {
		name string
		mode headerMode
	}{
		{"none", headerNone},
		{"presence", headerPresence},
		{"value", headerValue},
	}
	reqHeaders := http.Header{benchHeaderName: []string{"gold"}}
	for _, tc := range cases {
		engine, path := buildBenchEngine(b, routingTableSize, methodUnrestricted, tc.mode)
		entry := engine.Lookup(path)[0]
		preds := entry.Route.Match.Headers
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			var ok bool
			for i := 0; i < b.N; i++ {
				ok = HeadersMatch(preds, reqHeaders)
			}
			sinkBool = ok
		})
	}
}

// matchRequest reproduces the data-plane admission decision a request goes
// through after the listener hands off: path lookup, method admission against
// the first candidate, then header predicate evaluation. It returns the matched
// entry or nil. Kept allocation-free on purpose.
func matchRequest(engine *Engine, method string, path []byte, headers http.Header) *RouteIndexEntry {
	candidates := engine.Lookup(path)
	if len(candidates) == 0 {
		return nil
	}
	methodBit := requestMethodBit(method)
	for _, c := range candidates {
		if c.Route.Match.Methods&methodBit == 0 {
			continue
		}
		if !HeadersMatch(c.Route.Match.Headers, headers) {
			continue
		}
		return c
	}
	return nil
}

// BenchmarkRequestMatch is the headline enterprise benchmark: it walks the full
// orthogonal matrix of method and header policies through the complete matching
// pipeline. Use this with benchstat to gate routing-path performance in CI.
func BenchmarkRequestMatch(b *testing.B) {
	methodCases := []struct {
		name string
		mode methodMode
	}{
		{"methods=any", methodUnrestricted},
		{"methods=restricted", methodRestricted},
	}
	headerCases := []struct {
		name string
		mode headerMode
	}{
		{"headers=none", headerNone},
		{"headers=presence", headerPresence},
		{"headers=value", headerValue},
	}

	reqHeaders := http.Header{benchHeaderName: []string{"gold"}}

	for _, mc := range methodCases {
		for _, hc := range headerCases {
			engine, path := buildBenchEngine(b, routingTableSize, mc.mode, hc.mode)
			b.Run(mc.name+"/"+hc.name, func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				var matched *RouteIndexEntry
				for i := 0; i < b.N; i++ {
					matched = matchRequest(engine, http.MethodGet, path, reqHeaders)
				}
				if matched == nil {
					b.Fatal("expected a route match")
				}
				sinkEntries = []*RouteIndexEntry{matched}
			})
		}
	}
}

// BenchmarkRequestMatchParallel measures the same full pipeline under
// contention to surface shared-state or cache-line issues that only appear on
// multi-core production hosts. The matcher is read-only over the compiled
// snapshot, so this should scale linearly.
func BenchmarkRequestMatchParallel(b *testing.B) {
	engine, path := buildBenchEngine(b, routingTableSize, methodRestricted, headerValue)
	reqHeaders := http.Header{benchHeaderName: []string{"gold"}}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// The nil-check must live inside the loop: RunParallel may hand a
		// worker zero iterations, in which case a post-loop check would
		// observe an unset variable and report a false failure.
		for pb.Next() {
			if matchRequest(engine, http.MethodGet, path, reqHeaders) == nil {
				b.Error("expected a route match")
			}
		}
	})
}
