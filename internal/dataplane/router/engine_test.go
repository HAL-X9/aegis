package router

import (
	"strings"
	"testing"

	"github.com/aegis/internal/controlplane/snapshot"
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

	t.Run("builds engine and resolves route candidates", func(t *testing.T) {
		engine, err := BuildEngine(&snapshot.CompiledConfig{
			Routes: []snapshot.CompiledRoute{
				{
					Name: "api",
					Match: snapshot.CompiledMatch{
						PathPrefix: "/api",
					},
					Upstream: "http://localhost:8080",
				},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if engine == nil {
			t.Fatal("expected non-nil engine")
		}
		got := engine.Lookup([]byte("/api"))
		if len(got) != 1 || got[0].Route.Name != "api" {
			t.Fatalf("lookup result = %#v", got)
		}
	})
}

func TestEngineLookup(t *testing.T) {
	t.Run("nil receiver returns nil", func(t *testing.T) {
		var engine *Engine
		if got := engine.Lookup([]byte("/x")); got != nil {
			t.Fatalf("got %#v, want nil", got)
		}
	})

	t.Run("nil path returns nil", func(t *testing.T) {
		engine, err := BuildEngine(&snapshot.CompiledConfig{
			Routes: []snapshot.CompiledRoute{
				{
					Name: "x",
					Match: snapshot.CompiledMatch{
						PathPrefix: "/x",
					},
					Upstream: "http://h:1",
				},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := engine.Lookup(nil); got != nil {
			t.Fatalf("got %#v, want nil", got)
		}
	})
}
