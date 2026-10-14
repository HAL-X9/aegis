package router

import (
	"fmt"
	"net/url"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

// Engine is the request-ready routing view of a compiled snapshot. The
// control plane has already built everything it serves, so NewEngine only
// wraps it. Lookup, Route, UpstreamURL, and HeaderNames never allocate.
type Engine struct {
	index       snapshot.RouteIndex
	routes      []snapshot.CompiledRoute
	services    snapshot.CompiledServices
	headerNames snapshot.HeaderRegistry
}

// NewEngine wraps cfg for request-time use. The snapshot is immutable, so
// the Engine shares its slices instead of copying them.
func NewEngine(cfg *snapshot.CompiledConfig) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	return &Engine{
		index:       cfg.Index,
		routes:      cfg.Routes,
		services:    cfg.Services,
		headerNames: cfg.HeaderNames,
	}, nil
}

// Lookup calls visit, in priority order (static > param > wildcard, most
// specific first, including path_prefix fallbacks), for every candidate
// set that structurally matches path. visit should apply predicates
// Lookup can't — method, headers — and return true once it has accepted
// a route; Lookup stops as soon as that happens. See snapshot.RouteIndex.Lookup
// for the full contract.
func (e *Engine) Lookup(path string, visit func(candidates []snapshot.RouteID) bool) {
	if e == nil || path == "" {
		return
	}
	e.index.Lookup(path, visit)
}

func (e *Engine) Route(id snapshot.RouteID) *snapshot.CompiledRoute {
	return &e.routes[id]
}

func (e *Engine) UpstreamURL(route *snapshot.CompiledRoute) *url.URL {
	return e.services.Items[route.Service].Upstream
}

// HeaderNames returns the registry that resolves dynamic HeaderIDs
// (snapshot.HeaderDynamicStart and above) referenced by any route's
// header policy — see internal/dataplane/policy.ExecuteMutations.
// Well-known headers never need it.
func (e *Engine) HeaderNames() *snapshot.HeaderRegistry {
	return &e.headerNames
}
