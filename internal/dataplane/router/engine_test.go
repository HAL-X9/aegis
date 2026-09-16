package router

import (
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

func TestBuildEngine(t *testing.T) {
	t.Run("returns error for nil config", func(t *testing.T) {
		_, err := BuildEngine(nil)
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "config is nil") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns error for invalid upstream", func(t *testing.T) {
		cfg := &snapshot.CompiledConfig{
			Services: snapshot.CompiledServices{
				Items: []snapshot.CompiledService{
					{
						Name:     "api",
						Upstream: "://invalid-url",
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

		_, err := BuildEngine(cfg)
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "invalid upstream") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("returns error for invalid service reference", func(t *testing.T) {
		cfg := &snapshot.CompiledConfig{
			Services: snapshot.CompiledServices{
				Items: []snapshot.CompiledService{
					{
						Name:     "api",
						Upstream: "http://localhost:8080",
					},
				},
			},
			Routes: []snapshot.CompiledRoute{
				{
					Name:    "api",
					Service: snapshot.ServiceID(1),
					Match: snapshot.CompiledMatch{
						PathPrefix: "/api",
					},
				},
			},
		}

		_, err := BuildEngine(cfg)
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "references invalid service ID") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("builds engine and resolves route", func(t *testing.T) {
		cfg := &snapshot.CompiledConfig{
			Services: snapshot.CompiledServices{
				Items: []snapshot.CompiledService{
					{
						Name:     "api",
						Upstream: "http://localhost:8080",
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

		engine, err := BuildEngine(cfg)
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
		engine, err := BuildEngine(&snapshot.CompiledConfig{
			Services: snapshot.CompiledServices{
				Items: []snapshot.CompiledService{
					{
						Name:     "x",
						Upstream: "http://h:1",
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
		engine, err := BuildEngine(&snapshot.CompiledConfig{
			Services: snapshot.CompiledServices{
				Items: []snapshot.CompiledService{
					{
						Name:     "api",
						Upstream: "http://localhost:8080",
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
