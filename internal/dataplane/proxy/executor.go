package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/dataplane/policy"
	"github.com/HAL-X9/aegis/internal/dataplane/request"
	"github.com/HAL-X9/aegis/internal/dataplane/routelabel"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
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

// buildUpstreamRequest builds the outbound request for route, carrying
// over r's method, headers, body, and context, with the path/query
// rewritten onto the route's upstream origin.
//
// Two corrections versus a naive `*out = *r`:
//   - out.Header is r.Header.Clone(), not a shared reference. `*out = *r`
//     copies the Header map header, not its contents, so without cloning,
//     any header mutation on the outbound request (see policy.ExecuteMutations
//     below) would silently mutate the inbound request's headers too.
//   - out.Host is explicitly set to the upstream's host. net/http prefers
//     Request.Host over Request.URL.Host when sending a request; left
//     unset, it carries over the original client's Host header, and any
//     upstream that routes by Host would see the wrong one.
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
func (executor *Executor) buildUpstreamRequest(view *View, r *http.Request, route *snapshot.CompiledRoute) (req *http.Request, release func()) {
	upstreamURL := *view.Engine.UpstreamURL(route)
	upstreamURL.Path = r.URL.Path
	upstreamURL.RawPath = r.URL.RawPath
	upstreamURL.RawQuery = r.URL.RawQuery

	header := r.Header.Clone()

	if _, poolable := executor.transport.(*http.Transport); poolable {
		combo := upstreamRequestPool.Get().(*upstreamRequest)

		combo.url = upstreamURL
		// A full struct copy of *r — not field-by-field construction — is
		// what carries over r's unexported ctx (deadline, cancellation,
		// trace info) without an extra WithContext allocation: WithContext
		// just does this same copy internally and hands back a *new* heap
		// object, so doing it ourselves into combo.req is strictly one
		// allocation cheaper. Header and Host are then overwritten below.
		combo.req = *r
		combo.req.URL = &combo.url
		combo.req.Host = upstreamURL.Host
		combo.req.Header = header
		// RequestURI is populated by net/http for incoming server requests
		// and must be empty on outgoing client requests — Transport.RoundTrip
		// rejects it otherwise ("Request.RequestURI can't be set in client
		// requests"). Everything else copied from *r (Body, ContentLength,
		// TLS, etc.) is either correct as-is for a proxied request or
		// harmless/ignored by the client Transport.
		combo.req.RequestURI = ""

		return &combo.req, func() { upstreamRequestPool.Put(combo) }
	}

	out := new(http.Request)
	*out = *r
	out.URL = &upstreamURL
	out.Host = upstreamURL.Host
	out.Header = header
	out.RequestURI = ""
	return out, func() {}
}

// ServeHTTP resolves the incoming request using the routing engine and
// proxies it to a matching upstream service.
//
// Behavior:
//   - returns 503 if no View has been published yet
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
// never reach a route match (503/404/405 above) are left unlabeled and
// fall back to "unmatched" in the recorded metrics.
func (executor *Executor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	view := executor.view.Load()
	if view == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
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

	view.Engine.Lookup(r.URL.Path, func(candidateIDs []snapshot.RouteID) bool {
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

	req, release := executor.buildUpstreamRequest(view, r, matchedRoute)
	defer release()

	headerNames := view.Engine.HeaderNames()

	// Build the complete gateway-generated forwarding information before
	// applying the final request header policy. This ordering is
	// intentional: a policy such as `remove: X-Forwarded-For` must be able
	// to remove both a client-supplied value and an X-Forwarded-For value
	// generated by the gateway itself.
	request.SetForwardedHeaders(req.Header, r)

	policy.ExecuteMutations(req.Header, &matchedRoute.Policies.Headers.Request, headerNames)

	// Hop-by-hop headers must never reach the upstream, regardless of
	// whether they came from the client, a forwarding helper, or a policy.
	request.RemoveHopHeaders(req.Header)

	resp, err := executor.transport.RoundTrip(req)
	if err != nil {
		http.Error(w, "bad gateway: upstream request failed", http.StatusBadGateway)
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
