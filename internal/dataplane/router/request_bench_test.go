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
// Lookup itself is benchmarked under three deliberately different route
// table shapes, each isolating one structural variable:
//
//	HighFanout — static routes only, single high-fanout branch point
//	             (stresses child-scan width at one node)
//	Deep       — static routes only, uniform depth with moderate,
//	             gradual branching (stresses traversal depth)
//	Mixed      — static, :param, and *wildcard routes interleaved under
//	             shared prefixes (stresses all three edge kinds
//	             coexisting, closest to a real gateway config)
//
// Every benchmark calls b.ReportAllocs(); the matching path is expected to be
// allocation-free (0 allocs/op). A non-zero alloc count is a regression and
// should fail review even if ns/op looks acceptable.
//
// Inputs that vary per call (method, header value) are cycled across a small
// fixed set on every iteration rather than held constant. A constant input
// lets the compiler hoist the computation out of the loop and lets the CPU's
// branch predictor achieve a 100% hit rate, both of which understate real
// cost — sub-nanosecond results on a simple bitwise check are the tell.
// Cycling the input keeps the work inside the loop honest without
// introducing any additional allocation.
//
// Run:
//
//	go test ./internal/dataplane/router/ -run '^$' -bench 'Request|Lookup|Mixed|Headers|Method' -benchmem
//	go test ./internal/dataplane/router/ -run '^$' -bench Request -benchmem -benchtime 3s -count 10 > before.txt
//	benchstat old.txt before.txt
//
// Pin the measurement environment for comparable numbers:
//
//	GOMAXPROCS=1 GOGC=off go test ... -benchmem -count 10

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
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

// benchMethods cycles requests across multiple HTTP methods so method
// admission is re-evaluated on real, changing input each iteration instead
// of a single constant the compiler or branch predictor could special-case.
var benchMethods = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodDelete,
}

// benchHeaderValues cycles the incoming header value across allowed and
// disallowed values for the same reason: HeadersMatch's outcome (and the
// number of comparisons it performs before returning) must vary between
// iterations for the benchmark to reflect real traffic.
var benchHeaderValues = []string{"bronze", "silver", "gold", "platinum", "unranked"}

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
	services := make([]snapshot.CompiledService, 0, size)

	for i := 0; i < size; i++ {
		id := strconv.Itoa(i)

		services = append(services, snapshot.CompiledService{
			Name:     "svc-" + id,
			Upstream: "http://upstream-" + id + ".internal:8080",
		})

		routes = append(routes, snapshot.CompiledRoute{
			Name:    "svc-" + id,
			Service: snapshot.ServiceID(i),
			Match: snapshot.CompiledMatch{
				PathPrefix: "/api/v1/service-" + id + "/resource",
				Methods:    mask,
				Headers:    preds,
			},
		})
	}

	engine, err := BuildEngine(&snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{
			Items: services,
		},
		Routes: routes,
	})
	if err != nil {
		tb.Fatalf("BuildEngine: %v", err)
	}

	target := strconv.Itoa(size / 2)
	path := []byte("/api/v1/service-" + target + "/resource")

	if got := engine.Lookup(path); len(got) == 0 {
		tb.Fatalf("fixture path %q did not resolve to a route", path)
	}

	return engine, path
}

func buildDeepBenchEngine(tb testing.TB, size int) (*Engine, []byte) {
	tb.Helper()

	// We deliberately construct a deep and moderately branching routing tree.
	//
	// The current router compares static children linearly, so this benchmark
	// exposes the cost of child fan-out independently from the shallow
	// high-fanout benchmark above.
	const (
		depth   = 8
		fanout  = 4
		maxLeaf = 4096
	)

	if size > maxLeaf {
		size = maxLeaf
	}

	routes := make([]snapshot.CompiledRoute, 0, size)
	services := make([]snapshot.CompiledService, 0, size)

	// Generate paths such as:
	//
	// /api/v1/tenant-03/region-02/service-01/resource-03/version-00/endpoint-01
	//
	// The exact names are not important. What matters is that routes share
	// several prefix levels and branch gradually instead of creating one
	// enormous fan-out at a single node.
	for i := 0; i < size; i++ {
		n := i

		segments := make([]string, depth)

		for level := 0; level < depth; level++ {
			value := n % fanout
			n /= fanout

			segments[level] = "s" + strconv.Itoa(value)
		}

		path := "/api/v1"
		for _, segment := range segments {
			path += "/" + segment
		}

		id := strconv.Itoa(i)

		services = append(services, snapshot.CompiledService{
			Name:     "deep-svc-" + id,
			Upstream: "http://deep-upstream-" + id + ".internal:8080",
		})

		routes = append(routes, snapshot.CompiledRoute{
			Name:    "deep-route-" + id,
			Service: snapshot.ServiceID(i),
			Match: snapshot.CompiledMatch{
				PathPrefix: path,
				Methods:    methodmask.MethodAll,
			},
		})
	}

	engine, err := BuildEngine(&snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{
			Items: services,
		},
		Routes: routes,
	})
	if err != nil {
		tb.Fatalf("BuildEngine: %v", err)
	}

	// Use the last generated route as the lookup target.
	n := size - 1

	segments := make([]string, depth)

	for level := 0; level < depth; level++ {
		value := n % fanout
		n /= fanout

		segments[level] = "s" + strconv.Itoa(value)
	}

	pathString := "/api/v1"
	for _, segment := range segments {
		pathString += "/" + segment
	}

	path := []byte(pathString)

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

// BenchmarkLookupHighFanout measures the radix path lookup in isolation across table
// sizes. This is the floor cost every request pays regardless of policy.
func BenchmarkLookupHighFanout(b *testing.B) {
	for _, size := range []int{16, 64, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768} {
		engine, path := buildBenchEngine(b, size, methodUnrestricted, headerNone)

		nodes, maxChildren, avgChildren := trieStats(engine.trie.root)

		b.Logf(
			"routes=%d nodes=%d maxChildren=%d avgChildren=%.2f",
			size,
			nodes,
			maxChildren,
			avgChildren,
		)

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

func BenchmarkLookupDeep(b *testing.B) {
	// buildDeepBenchEngine caps at maxLeaf=4096 (depth=8, fanout=4 exhausts
	// its 4^8=65536 address space long before that, but 4096 is where the
	// fixture stops growing meaningfully). Sizes beyond that would silently
	// re-benchmark the same 4096-leaf tree under a misleading larger label,
	// so the size list stops there instead of implying scale that isn't
	// actually being exercised.
	for _, size := range []int{16, 64, 256, 512, 1024, 2048, 4096} {
		engine, path := buildDeepBenchEngine(b, size)

		nodes, maxChildren, avgChildren := trieStats(engine.trie.root)

		b.Logf(
			"routes=%d nodes=%d maxChildren=%d avgChildren=%.2f",
			size,
			nodes,
			maxChildren,
			avgChildren,
		)

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

// buildMixedBenchEngine constructs a heterogeneous routing table resembling
// a real gateway config: a blend of purely static routes, routes with one or
// more `:param` segments, and routes terminated by a `*wildcard` segment,
// interleaved at varying depths rather than segregated into their own
// subtrees. This exercises all three edge kinds (static, paramChild,
// wildcardChild) coexisting under shared parents, which neither
// BenchmarkLookupHighFanout (static-only, single branch point) nor
// BenchmarkLookupDeep (static-only, uniform depth) covers.
//
// It returns the engine plus a fixed set of lookup paths, one per route
// pattern group, so BenchmarkLookupMixed can cycle across all three edge
// kinds instead of only ever exercising one.
func buildMixedBenchEngine(tb testing.TB, groups int) (*Engine, [][]byte) {
	tb.Helper()

	routes := make([]snapshot.CompiledRoute, 0, groups*3)
	services := make([]snapshot.CompiledService, 0, groups*3)
	lookupPaths := make([][]byte, 0, groups*3)

	addRoute := func(name, pathPrefix, lookupPath string) {
		idx := len(services)
		services = append(services, snapshot.CompiledService{
			Name:     name,
			Upstream: "http://upstream-" + name + ".internal:8080",
		})
		routes = append(routes, snapshot.CompiledRoute{
			Name:    name,
			Service: snapshot.ServiceID(idx),
			Match: snapshot.CompiledMatch{
				PathPrefix: pathPrefix,
				Methods:    methodmask.MethodAll,
			},
		})
		lookupPaths = append(lookupPaths, []byte(lookupPath))
	}

	for i := 0; i < groups; i++ {
		id := strconv.Itoa(i)

		// Static: fully fixed path, no dynamic segments.
		addRoute(
			"static-"+id,
			"/api/v1/tenants/"+id+"/health",
			"/api/v1/tenants/"+id+"/health",
		)

		// Param: one dynamic segment nested among static ones, sharing the
		// "/api/v1/tenants/{id}" static prefix with the static group above
		// so the two edge kinds actually meet at a common parent node.
		addRoute(
			"param-"+id,
			"/api/v1/tenants/"+id+"/users/:userID/profile",
			"/api/v1/tenants/"+id+"/users/u-"+id+"/profile",
		)

		// Wildcard: terminates the route, consuming an arbitrary remainder,
		// again branching from the same tenant-scoped static prefix.
		addRoute(
			"wildcard-"+id,
			"/api/v1/tenants/"+id+"/assets/*rest",
			"/api/v1/tenants/"+id+"/assets/img/2026/banner.png",
		)
	}

	engine, err := BuildEngine(&snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{Items: services},
		Routes:   routes,
	})
	if err != nil {
		tb.Fatalf("BuildEngine: %v", err)
	}

	for _, p := range lookupPaths {
		if got := engine.Lookup(p); len(got) == 0 {
			tb.Fatalf("fixture path %q did not resolve to a route", p)
		}
	}

	return engine, lookupPaths
}

// BenchmarkLookupMixed measures lookup cost against a heterogeneous routing
// table where static, param, and wildcard routes are interleaved under
// shared prefixes, and the lookup path cycles across all three edge kinds
// on every iteration. This is the closest of the three lookup benchmarks to
// an actual gateway config, where route shape is never uniform the way
// BenchmarkLookupHighFanout and BenchmarkLookupDeep deliberately hold it.
func BenchmarkLookupMixed(b *testing.B) {
	for _, groups := range []int{16, 64, 256, 1024, 4096} {
		engine, paths := buildMixedBenchEngine(b, groups)

		nodes, maxChildren, avgChildren := trieStats(engine.trie.root)
		b.Logf(
			"groups=%d (routes=%d) nodes=%d maxChildren=%d avgChildren=%.2f",
			groups, groups*3, nodes, maxChildren, avgChildren,
		)

		b.Run("routes="+strconv.Itoa(groups*3), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			var out []*RouteIndexEntry
			for i := 0; i < b.N; i++ {
				out = engine.Lookup(paths[i%len(paths)])
			}
			sinkEntries = out
		})
	}
}

func trieStats(root *RadixNode) (nodes int, maxChildren int, avgChildren float64) {
	if root == nil {
		return 0, 0, 0
	}

	totalChildren := 0

	var walk func(*RadixNode)

	walk = func(node *RadixNode) {
		nodes++

		children := len(node.children)
		totalChildren += children

		if children > maxChildren {
			maxChildren = children
		}

		for _, child := range node.children {
			walk(child)
		}
	}

	walk(root)

	avgChildren = float64(totalChildren) / float64(nodes)

	return
}

// BenchmarkMethodAdmission measures only the method resolution + bitmask check,
// contrasting an unrestricted route (MethodAll) against a restricted one.
//
// The request method is cycled across benchMethods on every iteration. A
// fixed method would let the branch predictor and, for the bitwise AND
// itself, the compiler treat the check as effectively constant, producing
// sub-nanosecond results that don't reflect real per-request cost.
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
				method := benchMethods[i%len(benchMethods)]
				ok = entry.Route.Match.Methods&requestMethodBit(method) != 0
			}
			sinkBool = ok
		})
	}
}

// BenchmarkHeadersMatch measures header predicate evaluation in isolation for
// the three supported header modes.
//
// The incoming header value is cycled across benchHeaderValues, including
// values outside the allowed set, so both the match and no-match paths are
// exercised and the compiler cannot fold the comparison to a constant.
func BenchmarkHeadersMatch(b *testing.B) {
	cases := []struct {
		name string
		mode headerMode
	}{
		{"none", headerNone},
		{"presence", headerPresence},
		{"value", headerValue},
	}
	for _, tc := range cases {
		engine, path := buildBenchEngine(b, routingTableSize, methodUnrestricted, tc.mode)
		entry := engine.Lookup(path)[0]
		preds := entry.Route.Match.Headers

		reqHeaders := make([]http.Header, len(benchHeaderValues))
		for i, v := range benchHeaderValues {
			reqHeaders[i] = http.Header{benchHeaderName: []string{v}}
		}

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			var ok bool
			for i := 0; i < b.N; i++ {
				ok = HeadersMatch(preds, reqHeaders[i%len(reqHeaders)])
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
//
// The request method cycles across benchMethods (GET is always admitted by
// the fixture route, so the matched/unmatched outcome for the method stage
// itself stays representative of an accepted request while still forcing a
// real per-iteration comparison rather than a hoistable constant). The header
// value cycles across the allowed set so HeadersMatch performs a genuine
// comparison against a changing input on every call.
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

	allowedHeaderValues := []string{"bronze", "silver", "gold", "platinum"}

	for _, mc := range methodCases {
		for _, hc := range headerCases {
			engine, path := buildBenchEngine(b, routingTableSize, mc.mode, hc.mode)

			reqHeaders := make([]http.Header, len(allowedHeaderValues))
			for i, v := range allowedHeaderValues {
				reqHeaders[i] = http.Header{benchHeaderName: []string{v}}
			}

			b.Run(mc.name+"/"+hc.name, func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				var matched *RouteIndexEntry
				for i := 0; i < b.N; i++ {
					h := reqHeaders[i%len(reqHeaders)]
					matched = matchRequest(engine, http.MethodGet, path, h)
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

	allowedHeaderValues := []string{"bronze", "silver", "gold", "platinum"}
	reqHeaders := make([]http.Header, len(allowedHeaderValues))
	for i, v := range allowedHeaderValues {
		reqHeaders[i] = http.Header{benchHeaderName: []string{v}}
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// Each goroutine gets its own counter so cycling through reqHeaders
		// doesn't require shared, contended state.
		i := 0
		// The nil-check must live inside the loop: RunParallel may hand a
		// worker zero iterations, in which case a post-loop check would
		// observe an unset variable and report a false failure.
		for pb.Next() {
			h := reqHeaders[i%len(reqHeaders)]
			i++
			if matchRequest(engine, http.MethodGet, path, h) == nil {
				b.Error("expected a route match")
			}
		}
	})
}
