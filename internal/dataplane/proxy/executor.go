package proxy

import (
	"io"
	"maps"
	"net/http"
	"net/url"
	"sync"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
	"github.com/HAL-X9/aegis/internal/dataplane/policy"
	"github.com/HAL-X9/aegis/internal/dataplane/request"
	"github.com/HAL-X9/aegis/internal/dataplane/routelabel"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
)

// Executor implements an HTTP reverse proxy handler backed by a routing engine.
//
// It resolves incoming requests against a routing index and forwards matched
// requests to upstream services using the configured RoundTripper.
type Executor struct {
	// engine provides fast path-based route lookup.
	engine *router.Engine

	// rateLimiters enforces the per-route rate-limit policy compiled into
	// the snapshot.
	rateLimiters *policy.RateLimiterSet

	// transport is responsible for executing outbound HTTP requests.
	// It must be non-nil and safe for concurrent use.
	transport http.RoundTripper
}

var copyBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

// upstreamRequest bundles the outbound *http.Request and its *url.URL in a
// single heap allocation. RoundTripper requires req.URL to be a pointer, so
// without this trick the two would land in two separate mallocgc calls.
//
// Pooling this is only safe because the standard library's *http.Transport
// documents that it never retains a *Request or its URL past the RoundTrip
// call that owns it. Executor.transport is an interface, so that contract
// cannot be assumed for whatever value is actually injected — buildUpstreamRequest
// below type-asserts to *http.Transport and only pools on that positive
// match, falling back to a plain allocation for any other RoundTripper
// (e.g. one that queues requests for background retry).
type upstreamRequest struct {
	req http.Request
	url url.URL
}

var upstreamRequestPool = sync.Pool{
	New: func() any { return new(upstreamRequest) },
}

// NewExecutor creates a new Executor instance.
// engine, rateLimiters, and transport are all required for correct operation.
func NewExecutor(engine *router.Engine, rateLimiters *policy.RateLimiterSet, transport http.RoundTripper) *Executor {
	return &Executor{
		engine:       engine,
		rateLimiters: rateLimiters,
		transport:    transport,
	}
}

// buildUpstreamRequest builds the outbound request for route, carrying
// over r's method, headers, body, and context, with the path/query
// rewritten onto the route's upstream origin.
//
// The req+url pair is served from upstreamRequestPool only when transport
// is verified to be the standard library's *http.Transport — the only
// RoundTripper whose documented behavior makes reusing that memory after
// RoundTrip returns safe (see upstreamRequestPool's doc comment). Any
// other transport gets a freshly allocated, unpooled request instead, so
// pooling can never be silently unsafe just because Executor was built
// with a different RoundTripper.
//
// release must be called exactly once when req is no longer needed.
func (executor *Executor) buildUpstreamRequest(r *http.Request, route *snapshot.CompiledRoute) (req *http.Request, release func()) {
	upstreamURL := *executor.engine.UpstreamURL(route)
	upstreamURL.Path = r.URL.Path
	upstreamURL.RawPath = r.URL.RawPath
	upstreamURL.RawQuery = r.URL.RawQuery

	if _, poolable := executor.transport.(*http.Transport); poolable {
		combo := upstreamRequestPool.Get().(*upstreamRequest)

		combo.url = upstreamURL
		// A full struct copy of *r — not field-by-field construction — is
		// what carries over r's unexported ctx (deadline, cancellation,
		// trace info) without an extra WithContext allocation: WithContext
		// just does this same copy internally and hands back a *new* heap
		// object, so doing it ourselves into combo.req is strictly one
		// allocation cheaper.
		combo.req = *r
		combo.req.URL = &combo.url
		// RequestURI is populated by net/http for incoming server requests
		// and must be empty on outgoing client requests — Transport.RoundTrip
		// rejects it otherwise ("Request.RequestURI can't be set in client
		// requests"). Everything else copied from *r (Header, Body,
		// ContentLength, TLS, etc.) is either correct as-is for a proxied
		// request or harmless/ignored by the client Transport.
		combo.req.RequestURI = ""

		return &combo.req, func() { upstreamRequestPool.Put(combo) }
	}

	out := new(http.Request)
	*out = *r
	out.URL = &upstreamURL
	out.RequestURI = ""
	return out, func() {}
}

// ServeHTTP resolves the incoming request using the routing engine and
// proxies it to a matching upstream service.
//
// Behavior:
//   - returns 503 if the routing engine is unavailable
//   - returns 500 if the transport is unavailable
//   - returns 404 if no route matches the request path
//   - returns 405 if no route supports the request method
//   - returns 429 if the matched route's rate-limit policy rejects the request
//   - returns 502 if upstream request execution fails
//
// The request body is forwarded as-is to the upstream service.
//
// Once a route is matched, ServeHTTP reports the route's identifier via
// routelabel so the metrics middleware can label request metrics by route
// instead of by raw path (see internal/dataplane/routelabel). Requests that
// never reach a route match (503/500/404/405 above) are left unlabeled and
// fall back to "unmatched" in the recorded metrics.
func (executor *Executor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Validate routing engine availability before request processing.
	if executor.engine == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	// Validate transport availability required for upstream communication.
	if executor.transport == nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Resolve the bitmask representation of the incoming HTTP method.
	// Unsupported methods are rejected explicitly.
	methodBit, ok := methodmask.MethodBit(r.Method)
	if !ok {
		http.Error(w, "unsupported HTTP method", http.StatusMethodNotAllowed)
		return
	}

	// pathMatched and methodMatched are tracked across every candidate set
	// the trie offers, not just the first (most specific) one. Lookup
	// tries lower-priority branches (static -> param -> wildcard, plus
	// path_prefix fallbacks) whenever visit returns false, so a route
	// that structurally matches but fails method/header predicates never
	// hides a valid fallback route — see docs/routing-and-errors.md.
	var (
		pathMatched   bool
		methodMatched bool
		matchedRoute  *snapshot.CompiledRoute
	)

	executor.engine.Lookup(r.URL.Path, func(candidateIDs []snapshot.RouteID) bool {
		pathMatched = true

		for _, id := range candidateIDs {
			route := executor.engine.Route(id)
			if route.Match.Methods&methodBit == 0 {
				continue
			}
			methodMatched = true

			if router.HeadersMatch(route.Match.Headers, r.Header) {
				matchedRoute = route
				return true
			}
		}
		return false
	})

	switch {
	case !pathMatched:
		http.NotFound(w, r)
		return
	case matchedRoute == nil && methodMatched:
		// Some candidate supported the method but none had matching
		// headers — from the client's perspective there is no route here.
		http.NotFound(w, r)
		return
	case matchedRoute == nil:
		http.Error(w, "method not allowed for matched route", http.StatusMethodNotAllowed)
		return
	}

	// Report the matched route to the metrics middleware, if present, so
	// even a subsequent 429/502 below still gets labeled with the route
	// that produced it rather than falling back to "unmatched".
	//
	// matchedRoute.Name is the route's stable, config-provided identifier
	// (docs/policies.md, `routes: - name: example`) — that's a contract
	// owned by snapshot.CompiledRoute, not an inference made here. An
	// empty Name would mean that contract was violated upstream (loader
	// or compiler bug), so metrics get an explicit sentinel instead of a
	// silently blank route label.
	routeLabel := matchedRoute.Name
	if routeLabel == "" {
		routeLabel = "unnamed-route"
	}
	routelabel.FromContext(r.Context()).SetRoutePattern(routeLabel)

	if !executor.rateLimiters.Allow(matchedRoute) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	req, release := executor.buildUpstreamRequest(r, matchedRoute)
	defer release()

	policy.ExecuteMutations(req.Header, &matchedRoute.Policies.Headers.Request)
	request.RemoveHopHeaders(req.Header)

	resp, err := executor.transport.RoundTrip(req)
	if err != nil {
		http.Error(w, "bad gateway: upstream request failed", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	policy.ExecuteMutations(resp.Header, &matchedRoute.Policies.Headers.Response)
	request.RemoveHopHeaders(resp.Header)
	maps.Copy(w.Header(), resp.Header)

	w.WriteHeader(resp.StatusCode)

	buf := copyBufferPool.Get().(*[]byte)
	defer copyBufferPool.Put(buf)

	_, _ = io.CopyBuffer(w, resp.Body, *buf)
}
