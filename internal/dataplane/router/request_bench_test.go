package router

// The benchmark suite measures the per-request matching pipeline:
//
//   1. radix path lookup
//   2. HTTP method admission
//   3. header predicate evaluation
//
// The lookup benchmarks isolate different trie shapes:
//
//   HighFanout — static routes with high fan-out at a branch point.
//   Deep       — static routes with greater traversal depth.
//   Mixed      — static, :param and *wildcard routes sharing prefixes.
//
// The request-path benchmarks use string paths, matching the production
// router API exactly.
//
// All hot-path benchmarks call ReportAllocs(). The expected allocation count
// for the router matching path is zero. Benchmark fixture construction is
// intentionally outside the timed region.
//
// Run:
//
//   go test ./internal/dataplane/router/ -run '^$' \
//     -bench 'Request|Lookup|Mixed|Headers|Method' -benchmem
//
// For before/after comparisons:
//
//   go test ./internal/dataplane/router/ -run '^$' \
//     -bench Request -benchmem -benchtime 3s -count 10 > before.txt
//
//   go test ./internal/dataplane/router/ -run '^$' \
//     -bench Request -benchmem -benchtime 3s -count 10 > after.txt
//
//   benchstat before.txt after.txt
//
// For lower measurement noise:
//
//   GOMAXPROCS=1 GOGC=off go test ... -benchmem -count 10

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

var (
	sinkEntries []*RouteIndexEntry
	sinkEntry   *RouteIndexEntry
	sinkBool    bool
)

const routingTableSize = 256

type headerMode int

const (
	headerNone headerMode = iota
	headerPresence
	headerValue
)

type methodMode int

const (
	methodUnrestricted methodMode = iota
	methodRestricted
)

var benchHeaderName = http.CanonicalHeaderKey("X-Tenant-Tier")

var benchMethods = []string{
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodDelete,
}

var benchHeaderValues = []string{
	"bronze",
	"silver",
	"gold",
	"platinum",
	"unranked",
}

func buildBenchEngine(
	tb testing.TB,
	size int,
	mm methodMode,
	hm headerMode,
) (*Engine, string) {
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
		preds = []snapshot.HeaderPredicate{
			{
				Name: benchHeaderName,
			},
		}

	case headerValue:
		preds = []snapshot.HeaderPredicate{
			{
				Name: benchHeaderName,
				AllowedValues: []string{
					"bronze",
					"silver",
					"gold",
					"platinum",
				},
			},
		}
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
	path := "/api/v1/service-" + target + "/resource"

	if got := engine.Lookup(path); len(got) == 0 {
		tb.Fatalf("fixture path %q did not resolve to a route", path)
	}

	return engine, path
}

func buildDeepBenchEngine(tb testing.TB, size int) (*Engine, string) {
	tb.Helper()

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

	n := size - 1

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

	if got := engine.Lookup(path); len(got) == 0 {
		tb.Fatalf("fixture path %q did not resolve to a route", path)
	}

	return engine, path
}

func requestMethodBit(method string) methodmask.MethodMask {
	bit, _ := methodmask.MethodBit(method)
	return bit
}

func BenchmarkLookupHighFanout(b *testing.B) {
	for _, size := range []int{
		16,
		64,
		256,
		512,
		1024,
		2048,
		4096,
		8192,
		16384,
		32768,
	} {
		engine, path := buildBenchEngine(
			b,
			size,
			methodUnrestricted,
			headerNone,
		)

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
	for _, size := range []int{
		16,
		64,
		256,
		512,
		1024,
		2048,
		4096,
	} {
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

func buildMixedBenchEngine(
	tb testing.TB,
	groups int,
) (*Engine, []string) {
	tb.Helper()

	routes := make([]snapshot.CompiledRoute, 0, groups*3)
	services := make([]snapshot.CompiledService, 0, groups*3)
	lookupPaths := make([]string, 0, groups*3)

	addRoute := func(
		name string,
		pathPrefix string,
		lookupPath string,
	) {
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

		lookupPaths = append(lookupPaths, lookupPath)
	}

	for i := 0; i < groups; i++ {
		id := strconv.Itoa(i)

		addRoute(
			"static-"+id,
			"/api/v1/tenants/"+id+"/health",
			"/api/v1/tenants/"+id+"/health",
		)

		addRoute(
			"param-"+id,
			"/api/v1/tenants/"+id+"/users/:userID/profile",
			"/api/v1/tenants/"+id+"/users/u-"+id+"/profile",
		)

		addRoute(
			"wildcard-"+id,
			"/api/v1/tenants/"+id+"/assets/*rest",
			"/api/v1/tenants/"+id+"/assets/img/2026/banner.png",
		)
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

	for _, path := range lookupPaths {
		if got := engine.Lookup(path); len(got) == 0 {
			tb.Fatalf("fixture path %q did not resolve to a route", path)
		}
	}

	return engine, lookupPaths
}

func BenchmarkLookupMixed(b *testing.B) {
	for _, groups := range []int{
		16,
		64,
		256,
		1024,
		4096,
	} {
		engine, paths := buildMixedBenchEngine(b, groups)

		nodes, maxChildren, avgChildren := trieStats(engine.trie.root)

		b.Logf(
			"groups=%d routes=%d nodes=%d maxChildren=%d avgChildren=%.2f",
			groups,
			groups*3,
			nodes,
			maxChildren,
			avgChildren,
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

func trieStats(root *RadixNode) (
	nodes int,
	maxChildren int,
	avgChildren float64,
) {
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

func BenchmarkMethodAdmission(b *testing.B) {
	cases := []struct {
		name string
		mode methodMode
	}{
		{
			name: "unrestricted",
			mode: methodUnrestricted,
		},
		{
			name: "restricted",
			mode: methodRestricted,
		},
	}

	for _, tc := range cases {
		engine, path := buildBenchEngine(
			b,
			routingTableSize,
			tc.mode,
			headerNone,
		)

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

func BenchmarkHeadersMatch(b *testing.B) {
	cases := []struct {
		name string
		mode headerMode
	}{
		{
			name: "none",
			mode: headerNone,
		},
		{
			name: "presence",
			mode: headerPresence,
		},
		{
			name: "value",
			mode: headerValue,
		},
	}

	for _, tc := range cases {
		engine, path := buildBenchEngine(
			b,
			routingTableSize,
			methodUnrestricted,
			tc.mode,
		)

		entry := engine.Lookup(path)[0]
		preds := entry.Route.Match.Headers

		reqHeaders := make([]http.Header, len(benchHeaderValues))

		for i, value := range benchHeaderValues {
			reqHeaders[i] = http.Header{
				benchHeaderName: []string{value},
			}
		}

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			var ok bool

			for i := 0; i < b.N; i++ {
				ok = HeadersMatch(
					preds,
					reqHeaders[i%len(reqHeaders)],
				)
			}

			sinkBool = ok
		})
	}
}

func matchRequest(
	engine *Engine,
	method string,
	path string,
	headers http.Header,
) *RouteIndexEntry {
	candidates := engine.Lookup(path)

	if len(candidates) == 0 {
		return nil
	}

	methodBit := requestMethodBit(method)

	for _, candidate := range candidates {
		if candidate.Route.Match.Methods&methodBit == 0 {
			continue
		}

		if !HeadersMatch(candidate.Route.Match.Headers, headers) {
			continue
		}

		return candidate
	}

	return nil
}

func BenchmarkRequestMatch(b *testing.B) {
	methodCases := []struct {
		name string
		mode methodMode
	}{
		{
			name: "methods=any",
			mode: methodUnrestricted,
		},
		{
			name: "methods=restricted",
			mode: methodRestricted,
		},
	}

	headerCases := []struct {
		name string
		mode headerMode
	}{
		{
			name: "headers=none",
			mode: headerNone,
		},
		{
			name: "headers=presence",
			mode: headerPresence,
		},
		{
			name: "headers=value",
			mode: headerValue,
		},
	}

	allowedHeaderValues := []string{
		"bronze",
		"silver",
		"gold",
		"platinum",
	}

	for _, mc := range methodCases {
		for _, hc := range headerCases {
			engine, path := buildBenchEngine(
				b,
				routingTableSize,
				mc.mode,
				hc.mode,
			)

			reqHeaders := make([]http.Header, len(allowedHeaderValues))

			for i, value := range allowedHeaderValues {
				reqHeaders[i] = http.Header{
					benchHeaderName: []string{value},
				}
			}

			b.Run(mc.name+"/"+hc.name, func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()

				var matched *RouteIndexEntry

				for i := 0; i < b.N; i++ {
					headers := reqHeaders[i%len(reqHeaders)]

					matched = matchRequest(
						engine,
						http.MethodGet,
						path,
						headers,
					)
				}

				if matched == nil {
					b.Fatal("expected a route match")
				}

				// Do not create []*RouteIndexEntry here.
				// The benchmark must not introduce an artificial allocation.
				sinkEntry = matched
			})
		}
	}
}

func BenchmarkRequestMatchParallel(b *testing.B) {
	engine, path := buildBenchEngine(
		b,
		routingTableSize,
		methodRestricted,
		headerValue,
	)

	allowedHeaderValues := []string{
		"bronze",
		"silver",
		"gold",
		"platinum",
	}

	reqHeaders := make([]http.Header, len(allowedHeaderValues))

	for i, value := range allowedHeaderValues {
		reqHeaders[i] = http.Header{
			benchHeaderName: []string{value},
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0

		for pb.Next() {
			headers := reqHeaders[i%len(reqHeaders)]
			i++

			if matchRequest(
				engine,
				http.MethodGet,
				path,
				headers,
			) == nil {
				b.Error("expected a route match")
			}
		}
	})
}
