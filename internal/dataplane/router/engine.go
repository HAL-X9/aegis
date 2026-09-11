package router

import (
	"fmt"
	"net/url"

	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

// Engine encapsulates the compiled, pointer-free routing structures used
// on the request hot path. Lookup, Route, and UpstreamURL never allocate.
type Engine struct {
	trie         *FlatTrie
	routes       []snapshot.CompiledRoute
	upstreamURLs []*url.URL // one entry per service, indexed by ServiceID
}

// BuildEngine compiles a control-plane snapshot into a request-ready
// Engine. This runs once per config reload — free to allocate, build an
// intermediate pointer tree, sort, etc. None of it runs on the request path.
func BuildEngine(cfg *snapshot.CompiledConfig) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	upstreamURLs := make([]*url.URL, len(cfg.Services.Items))
	for i, service := range cfg.Services.Items {
		u, err := url.Parse(service.Upstream)
		if err != nil {
			return nil, fmt.Errorf("service %q: invalid upstream %q: %w", service.Name, service.Upstream, err)
		}
		upstreamURLs[i] = u
	}

	for _, route := range cfg.Routes {
		if int(route.Service) >= len(cfg.Services.Items) {
			return nil, fmt.Errorf("route %q references invalid service ID %d", route.Name, route.Service)
		}
	}

	tree := BuildRadixTrie(cfg.Routes)

	trie, err := Flatten(tree)
	if err != nil {
		return nil, fmt.Errorf("flatten routing trie: %w", err)
	}

	return &Engine{
		trie:         trie,
		routes:       cfg.Routes,
		upstreamURLs: upstreamURLs,
	}, nil
}

// Lookup returns route-ID candidates matching path. The returned slice is
// a window into the engine's own arena — do not retain it past the
// engine's lifetime, and do not mutate it.
func (e *Engine) Lookup(path string) []uint32 {
	if e == nil || path == "" {
		return nil
	}
	return e.trie.Lookup(path)
}

// Route returns the compiled route for a route ID returned by Lookup.
func (e *Engine) Route(id uint32) *snapshot.CompiledRoute {
	return &e.routes[id]
}

// UpstreamURL returns the parsed upstream origin for route's service.
func (e *Engine) UpstreamURL(route *snapshot.CompiledRoute) *url.URL {
	return e.upstreamURLs[route.Service]
}
