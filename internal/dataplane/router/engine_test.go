package router

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/compile"
	"github.com/HAL-X9/aegis/internal/httpmethod"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

// buildEngine indexes cfg's routes the way the control plane does, then
// wraps the snapshot in an Engine.
func buildEngine(cfg *snapshot.CompiledConfig) (*Engine, error) {
	index, err := compile.Index(cfg.Routes)
	if err != nil {
		return nil, err
	}
	cfg.Index = index

	return NewEngine(cfg)
}

// testURL parses a literal upstream origin. A malformed literal is a bug in
// the test itself, so it panics rather than returning an error.
func testURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func TestNewEngine(t *testing.T) {
	t.Run("returns error for nil config", func(t *testing.T) {
		_, err := NewEngine(nil)
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "config is nil") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("builds engine and resolves route", func(t *testing.T) {
		cfg := &snapshot.CompiledConfig{
			Services: snapshot.CompiledServices{
				Items: []snapshot.CompiledService{
					{
						Name:     "api",
						Upstream: testURL("http://localhost:8080"),
					},
				},
			},
			Routes: []snapshot.CompiledRoute{
				{
					Name:    "api",
					Service: snapshot.ServiceID(0),
					Match: snapshot.CompiledMatch{
						PathPrefix: "/api",
					},
				},
			},
		}

		engine, err := buildEngine(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if engine == nil {
			t.Fatal("expected non-nil engine")
		}

		var ids []snapshot.RouteID

		engine.Lookup("/api", func(candidates []snapshot.RouteID) bool {
			ids = append(ids, candidates...)
			return true
		})

		if len(ids) != 1 {
			t.Fatalf("lookup result = %#v, want exactly one candidate", ids)
		}

		route := engine.Route(ids[0])
		if route == nil {
			t.Fatal("expected non-nil route")
		}

		if route.Name != "api" {
			t.Fatalf("route name = %q, want %q", route.Name, "api")
		}

		upstream := engine.UpstreamURL(route)
		if upstream == nil {
			t.Fatal("expected non-nil upstream URL")
		}

		if upstream.Scheme != "http" {
			t.Fatalf("upstream scheme = %q, want %q", upstream.Scheme, "http")
		}

		if upstream.Host != "localhost:8080" {
			t.Fatalf(
				"upstream host = %q, want %q",
				upstream.Host,
				"localhost:8080",
			)
		}
	})
}

func TestEngineLookup(t *testing.T) {
	t.Run("nil receiver does nothing", func(t *testing.T) {
		var engine *Engine

		called := false

		engine.Lookup("/x", func(candidates []snapshot.RouteID) bool {
			called = true
			return true
		})

		if called {
			t.Fatal("visit callback must not be called for nil engine")
		}
	})

	t.Run("empty path does nothing", func(t *testing.T) {
		engine, err := buildEngine(&snapshot.CompiledConfig{
			Services: snapshot.CompiledServices{
				Items: []snapshot.CompiledService{
					{
						Name:     "x",
						Upstream: testURL("http://h:1"),
					},
				},
			},
			Routes: []snapshot.CompiledRoute{
				{
					Name:    "x",
					Service: snapshot.ServiceID(0),
					Match: snapshot.CompiledMatch{
						PathPrefix: "/x",
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		called := false

		engine.Lookup("", func(candidates []snapshot.RouteID) bool {
			called = true
			return true
		})

		if called {
			t.Fatal("visit callback must not be called for empty path")
		}
	})

	t.Run("unmatched path does not produce candidates", func(t *testing.T) {
		engine, err := buildEngine(&snapshot.CompiledConfig{
			Services: snapshot.CompiledServices{
				Items: []snapshot.CompiledService{
					{
						Name:     "api",
						Upstream: testURL("http://localhost:8080"),
					},
				},
			},
			Routes: []snapshot.CompiledRoute{
				{
					Name:    "api",
					Service: snapshot.ServiceID(0),
					Match: snapshot.CompiledMatch{
						PathPrefix: "/api",
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		called := false

		engine.Lookup("/other", func(candidates []snapshot.RouteID) bool {
			called = true
			return true
		})

		if called {
			t.Fatal("visit callback must not be called for unmatched path")
		}
	})
}

// resolveRoute is one route of a Resolve test: a path prefix, the methods it
// supports, and an optional presence predicate on one header.
type resolveRoute struct {
	name    string
	prefix  string
	methods []string
	header  string
}

func resolveEngine(t *testing.T, routes ...resolveRoute) *Engine {
	t.Helper()

	cfg := &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{
			Items: []snapshot.CompiledService{{Name: "svc", Upstream: testURL("http://localhost:8080")}},
		},
	}
	for _, r := range routes {
		var mask httpmethod.Mask
		for _, method := range r.methods {
			bit, ok := httpmethod.Bit(method)
			if !ok {
				t.Fatalf("unknown method %q", method)
			}
			mask |= bit
		}

		match := snapshot.CompiledMatch{PathPrefix: r.prefix, Methods: mask}
		if r.header != "" {
			match.Headers = []snapshot.HeaderPredicate{{Name: r.header}}
		}
		cfg.Routes = append(cfg.Routes, snapshot.CompiledRoute{Name: r.name, Match: match})
	}

	engine, err := buildEngine(cfg)
	if err != nil {
		t.Fatalf("buildEngine: %v", err)
	}
	return engine
}

func TestResolve(t *testing.T) {
	get, post := []string{http.MethodGet}, []string{http.MethodPost}

	tests := map[string]struct {
		routes      []resolveRoute
		method      string
		path        string
		headers     http.Header
		wantOutcome Outcome
		wantRoute   string
	}{
		"path, method and header match": {
			routes:      []resolveRoute{{"profile", "/api/v1/profile", get, "Authorization"}},
			method:      http.MethodGet,
			path:        "/api/v1/profile/123",
			headers:     http.Header{"Authorization": {"Bearer t"}},
			wantOutcome: Matched,
			wantRoute:   "profile",
		},
		"unknown path is not found": {
			routes:      []resolveRoute{{"profile", "/api/v1/profile", get, ""}},
			method:      http.MethodGet,
			path:        "/does/not/exist",
			wantOutcome: NotFound,
		},
		"unsupported method is method not allowed": {
			routes:      []resolveRoute{{"profile", "/api/v1/profile", get, ""}},
			method:      http.MethodPost,
			path:        "/api/v1/profile",
			wantOutcome: MethodNotAllowed,
		},
		"missing header is not found, not method not allowed": {
			routes:      []resolveRoute{{"profile", "/api/v1/profile", get, "Authorization"}},
			method:      http.MethodGet,
			path:        "/api/v1/profile",
			wantOutcome: NotFound,
		},
		"failed header predicate falls back to a less specific route": {
			routes: []resolveRoute{
				{"report", "/files/report", get, "X-Tenant"},
				{"files", "/files", get, ""},
			},
			method:      http.MethodGet,
			path:        "/files/report/2026",
			wantOutcome: Matched,
			wantRoute:   "files",
		},
		"unsupported method falls back to a less specific route": {
			routes: []resolveRoute{
				{"upload", "/files/upload", post, ""},
				{"files", "/files", get, ""},
			},
			method:      http.MethodGet,
			path:        "/files/upload",
			wantOutcome: Matched,
			wantRoute:   "files",
		},
		"method not allowed when no candidate supports the method": {
			routes: []resolveRoute{
				{"upload", "/files/upload", post, ""},
				{"files", "/files", post, ""},
			},
			method:      http.MethodGet,
			path:        "/files/upload",
			wantOutcome: MethodNotAllowed,
		},
		"method supported only with a header that is absent is not found": {
			routes: []resolveRoute{
				{"upload", "/files/upload", post, ""},
				{"files", "/files", get, "X-Tenant"},
			},
			method:      http.MethodGet,
			path:        "/files/upload",
			wantOutcome: NotFound,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			engine := resolveEngine(t, tc.routes...)
			bit, _ := httpmethod.Bit(tc.method)

			route, outcome := engine.Resolve(tc.path, bit, tc.headers)

			if outcome != tc.wantOutcome {
				t.Fatalf("outcome = %d, want %d", outcome, tc.wantOutcome)
			}
			if tc.wantOutcome != Matched {
				if route != nil {
					t.Fatalf("route = %q, want none", route.Name)
				}
				return
			}
			if route == nil || route.Name != tc.wantRoute {
				t.Fatalf("route = %v, want %q", route, tc.wantRoute)
			}
		})
	}
}

func TestResolveOnNilEngineFindsNothing(t *testing.T) {
	var engine *Engine
	if route, outcome := engine.Resolve("/", httpmethod.Mask(1), nil); route != nil || outcome != NotFound {
		t.Fatalf("Resolve on a nil engine = (%v, %d), want (nil, NotFound)", route, outcome)
	}
}

// TestResolveDoesNotAllocate guards the documented property on every outcome.
func TestResolveDoesNotAllocate(t *testing.T) {
	engine := resolveEngine(t,
		resolveRoute{"report", "/files/report", []string{http.MethodGet}, "X-Tenant"},
		resolveRoute{"files", "/files", []string{http.MethodGet}, ""},
		resolveRoute{"upload", "/upload", []string{http.MethodPost}, ""},
	)
	get, _ := httpmethod.Bit(http.MethodGet)
	headers := http.Header{"X-Tenant": {"a"}}

	for name, path := range map[string]string{
		"matched":            "/files/report/1",
		"fallback":           "/files/other",
		"method not allowed": "/upload",
		"not found":          "/nowhere",
	} {
		allocs := testing.AllocsPerRun(100, func() { engine.Resolve(path, get, headers) })
		if allocs != 0 {
			t.Errorf("%s: Resolve allocated %v times per call, want 0", name, allocs)
		}
	}
}
