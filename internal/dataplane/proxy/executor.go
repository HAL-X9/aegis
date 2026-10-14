package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/HAL-X9/aegis/internal/dataplane/policy"
	"github.com/HAL-X9/aegis/internal/dataplane/request"
	"github.com/HAL-X9/aegis/internal/dataplane/retry"
	"github.com/HAL-X9/aegis/internal/dataplane/routelabel"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
	"github.com/HAL-X9/aegis/internal/httpmethod"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

// Executor implements an HTTP reverse proxy handler backed by a routing engine.
//
// It resolves incoming requests against a routing index and forwards matched
// requests to upstream services using the configured RoundTripper.
type Executor struct {
	// view holds the currently published View (routing engine + rate
	// limiters), swapped atomically by Publish. Load is safe for
	// concurrent use without a mutex on the request path.
	view atomic.Pointer[View]

	// transport is responsible for executing outbound HTTP requests.
	// Set once at construction; never swapped. Must be non-nil and safe
	// for concurrent use — NewExecutor enforces this so ServeHTTP doesn't
	// have to check it on every request.
	transport http.RoundTripper
}

var copyBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

// upstreamRequest holds an outbound request and its URL in a single
// allocation.
type upstreamRequest struct {
	req http.Request
	url url.URL
}

// NewExecutor creates an Executor bound to transport, which must be
// non-nil and safe for concurrent use — that's a constructor invariant,
// checked once here instead of on every request.
//
// The returned Executor serves 503 for every request until Publish is
// called at least once; call Publish with the initial View before handing
// the Executor to a server.
func NewExecutor(transport http.RoundTripper) (*Executor, error) {
	if transport == nil {
		return nil, fmt.Errorf("proxy: transport must not be nil")
	}
	return &Executor{transport: transport}, nil
}

// Publish atomically swaps in a new View. Callers build the complete View
// (Engine + Limiters) before calling Publish — there is no partial or
// in-place update, so a request mid-flight always sees either the whole
// old View or the whole new one, never a mix.
func (executor *Executor) Publish(v *View) {
	executor.view.Store(v)
}

// newUpstreamRequest returns a shallow copy of r with context ctx, addressed
// to route's upstream, with the canonical path, r's query, and a header
// built by request.UpstreamHeader. It keeps r's body, sets Host to the
// upstream host, and clears RequestURI, which client requests must not set.
func newUpstreamRequest(ctx context.Context, view *View, r *http.Request, route *snapshot.CompiledRoute, path string) *http.Request {
	out := &upstreamRequest{url: *view.Engine.UpstreamURL(route)}
	out.url.Path = path
	if path == r.URL.Path {
		// RawPath describes only the original path; a canonicalized path is
		// re-encoded from Path.
		out.url.RawPath = r.URL.RawPath
	}
	out.url.RawQuery = r.URL.RawQuery

	out.req = *r.WithContext(ctx)
	out.req.URL = &out.url
	out.req.Host = out.url.Host
	out.req.Header = request.UpstreamHeader(r)
	out.req.RequestURI = ""
	return &out.req
}

// ServeHTTP resolves the incoming request using the routing engine and
// proxies it to a matching upstream service.
//
// Behavior:
//   - returns 503 if no View has been published yet
//   - returns 400 if the request path has no canonical form
//     (see request.CanonicalPath); otherwise the canonical path is both
//     routed and forwarded
//   - returns 404 if no route matches the request path
//   - returns 405 if no route supports the request method
//   - returns 429 if the matched route's rate-limit policy rejects the request
//   - returns 504 if the upstream exchange times out (see isTimeout)
//   - returns 502 if upstream request execution fails otherwise
//   - retries idempotent body-less requests on transport errors and
//     502/503/504 when the route's service configures more than one attempt
//     (see internal/dataplane/retry); the rate limiter is checked once,
//     before the first attempt
//
// The request body is forwarded as-is to the upstream service.
//
// Once a route is matched, ServeHTTP reports the route's identifier via
// routelabel so the metrics middleware can label request metrics by route
// instead of by raw path (see internal/dataplane/routelabel). Requests that
// never reach a route match (503/404/405 above) are left unlabeled and
// fall back to "unmatched" in the recorded metrics.
func (executor *Executor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	view := executor.view.Load()
	if view == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	path, ok := request.CanonicalPath(r.URL)
	if !ok {
		http.Error(w, "bad request: invalid path", http.StatusBadRequest)
		return
	}

	// Resolve the bitmask representation of the incoming HTTP method.
	// Unsupported methods are rejected explicitly.
	methodBit, ok := httpmethod.Bit(r.Method)
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

	view.Engine.Lookup(path, func(candidateIDs []snapshot.RouteID) bool {
		pathMatched = true

		for _, id := range candidateIDs {
			route := view.Engine.Route(id)
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

	if !view.Limiters.Allow(matchedRoute) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	ctx := r.Context()
	if timeout := matchedRoute.Policies.Timeout; timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// The X-Forwarded-* headers set by newUpstreamRequest precede the
	// request policy, so the policy can override or remove them.
	req := newUpstreamRequest(ctx, view, r, matchedRoute, path)
	headerNames := view.Engine.HeaderNames()

	policy.ExecuteMutations(req.Header, &matchedRoute.Policies.Headers.Request, headerNames)

	// Hop-by-hop headers must never reach the upstream, regardless of
	// whether they came from the client, a forwarding helper, or a policy.
	request.RemoveHopHeaders(req.Header)

	// Every retry attempt reuses req as prepared above.
	resp, err := retry.Do(executor.transport, req, matchedRoute.Policies.Retry)
	if err != nil {
		if isTimeout(err) {
			http.Error(w, "gateway timeout: upstream did not respond in time", http.StatusGatewayTimeout)
		} else {
			http.Error(w, "bad gateway: upstream request failed", http.StatusBadGateway)
		}
		return
	}
	defer func() { _ = resp.Body.Close() }()

	policy.ExecuteMutations(resp.Header, &matchedRoute.Policies.Headers.Response, headerNames)
	request.RemoveHopHeaders(resp.Header)
	respHeader := w.Header()
	for name, values := range resp.Header {
		respHeader[name] = values
	}

	w.WriteHeader(resp.StatusCode)

	buf := copyBufferPool.Get().(*[]byte)
	defer copyBufferPool.Put(buf)

	_, _ = io.CopyBuffer(w, resp.Body, *buf)
}

// isTimeout reports whether err is a timeout: an expired service, attempt
// or listener deadline, or a transport dial or response-header timeout.
func isTimeout(err error) bool {
	var timeout interface{ Timeout() bool }
	return errors.As(err, &timeout) && timeout.Timeout()
}
