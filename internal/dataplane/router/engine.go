package router

import (
	"fmt"
	"net/url"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

// Engine encapsulates the compiled, pointer-free routing structures used
// on the request hot path. Lookup, Route, UpstreamURL, and HeaderNames
// never allocate.
type Engine struct {
	trie         *FlatTrie
	routes       []snapshot.CompiledRoute
	upstreamURLs []*url.URL // one entry per service, indexed by ServiceID
	headerNames  snapshot.HeaderRegistry
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
		headerNames:  cfg.HeaderNames,
	}, nil
}

// Lookup calls visit, in priority order (static > param > wildcard, most
// specific first, including path_prefix fallbacks), for every candidate
// set that structurally matches path. visit should apply predicates
// Lookup can't — method, headers — and return true once it has accepted
// a route; Lookup stops as soon as that happens. See router.FlatTrie.Lookup
// for the full contract.
func (e *Engine) Lookup(path string, visit func(candidates []snapshot.RouteID) bool) {
	if e == nil || path == "" {
		return
	}
	e.trie.Lookup(path, visit)
}

func (e *Engine) Route(id snapshot.RouteID) *snapshot.CompiledRoute {
	return &e.routes[id]
}

func (e *Engine) UpstreamURL(route *snapshot.CompiledRoute) *url.URL {
	return e.upstreamURLs[route.Service]
}

// HeaderNames returns the registry that resolves dynamic HeaderIDs
// (snapshot.HeaderDynamicStart and above) referenced by any route's
// header policy — see internal/dataplane/policy.ExecuteMutations.
// Well-known headers never need it.
func (e *Engine) HeaderNames() *snapshot.HeaderRegistry {
	return &e.headerNames
}
