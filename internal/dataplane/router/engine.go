package router

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/HAL-X9/aegis/internal/httpmethod"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

// Outcome is the result of resolving a request against the routes.
type Outcome uint8

const (
	// Matched means a route accepted the request.
	Matched Outcome = iota

	// NotFound means no route accepts the request: no route's path matches,
	// or only routes whose method is supported but whose header predicates
	// fail. To the client there is no route there.
	NotFound

	// MethodNotAllowed means some route's path matches, but none of the
	// matching routes supports the request method.
	MethodNotAllowed
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

// Route returns the compiled route that id, an identifier offered by Lookup,
// refers to. The result aliases the snapshot and must not be modified. id must
// come from this Engine's own index; an arbitrary value panics.
// Resolve selects the route for a request with the given canonical path,
// method bit and headers, and reports why there is none when it selects none.
// It never allocates.
//
// A route accepts the request when its path matches and it supports the
// method and its header predicates hold. The outcome is tracked across every
// candidate set the index offers, not just the first (most specific) one:
// Lookup tries lower-priority branches (static, then parameter, then wildcard,
// plus path_prefix fallbacks) whenever a candidate is rejected, so a route that
// matches by path but fails the method or header predicates never hides a valid
// fallback route (invariants I-006 and I-007 in docs/invariants.md; semantics
// in docs/routing-and-errors.md §1.4).
func (e *Engine) Resolve(path string, method httpmethod.Mask, headers http.Header) (*snapshot.CompiledRoute, Outcome) {
	var (
		pathMatched   bool
		methodMatched bool
		matched       *snapshot.CompiledRoute
	)

	e.Lookup(path, func(candidates []snapshot.RouteID) bool {
		pathMatched = true

		for _, id := range candidates {
			route := e.Route(id)
			if route.Match.Methods&method == 0 {
				continue
			}
			methodMatched = true

			if HeadersMatch(route.Match.Headers, headers) {
				matched = route
				return true
			}
		}
		return false
	})

	switch {
	case matched != nil:
		return matched, Matched
	case pathMatched && !methodMatched:
		return nil, MethodNotAllowed
	default:
		return nil, NotFound
	}
}

func (e *Engine) Route(id snapshot.RouteID) *snapshot.CompiledRoute {
	return &e.routes[id]
}

// UpstreamURL returns the parsed upstream origin (scheme and host) of the
// service route refers to. The result is shared by every request and must be
// copied before it is modified.
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
