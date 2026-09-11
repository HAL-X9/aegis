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
// Pooling is safe here because the standard http.Transport never retains a
// *Request or its URL past the RoundTrip call that owns it. If transport is
// ever swapped for a custom RoundTripper that queues requests for background
// retry, that contract must be re-verified before reusing this pool.
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

	candidateIDs := executor.engine.Lookup(r.URL.Path)
	if len(candidateIDs) == 0 {
		http.NotFound(w, r)
		return
	}

	// Resolve the bitmask representation of the incoming HTTP method.
	// Unsupported methods are rejected explicitly.
	methodBit, ok := methodmask.MethodBit(r.Method)
	if !ok {
		http.Error(w, "unsupported HTTP method", http.StatusMethodNotAllowed)
		return
	}

	var methodMatch bool
	var matchedRoute *snapshot.CompiledRoute

	for _, id := range candidateIDs {
		route := executor.engine.Route(id)
		if route.Match.Methods&methodBit != 0 {
			methodMatch = true

			if router.HeadersMatch(route.Match.Headers, r.Header) {
				matchedRoute = route
				break
			}
		}
	}

	if matchedRoute == nil {
		if !methodMatch {
			http.Error(w, "method not allowed for matched route", http.StatusMethodNotAllowed)
			return
		}
		http.NotFound(w, r)
		return
	}

	if !executor.rateLimiters.Allow(matchedRoute) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}

	combo := upstreamRequestPool.Get().(*upstreamRequest)
	defer upstreamRequestPool.Put(combo)

	combo.url = *executor.engine.UpstreamURL(matchedRoute)
	combo.url.Path = r.URL.Path
	combo.url.RawPath = r.URL.RawPath
	combo.url.RawQuery = r.URL.RawQuery

	// A full struct copy of *r — not field-by-field construction — is what
	// carries over r's unexported ctx (deadline, cancellation, trace info)
	// without an extra WithContext allocation: WithContext just does this
	// same copy internally and hands back a *new* heap object, so doing it
	// ourselves into combo.req is strictly one allocation cheaper.
	combo.req = *r
	combo.req.URL = &combo.url

	// RequestURI is populated by net/http for incoming server requests and
	// must be empty on outgoing client requests — Transport.RoundTrip
	// rejects it otherwise ("Request.RequestURI can't be set in client
	// requests"). Everything else copied from *r (Header, Body,
	// ContentLength, TLS, etc.) is either correct as-is for a proxied
	// request or harmless/ignored by the client Transport.
	combo.req.RequestURI = ""

	req := &combo.req

	policy.ExecuteMutations(req.Header, &matchedRoute.Policies.Headers.Request)
	request.RemoveHopHeaders(req.Header)

	resp, err := executor.transport.RoundTrip(req)
	if err != nil {
		http.Error(w, "bad gateway: upstream request failed", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	dstHeader := w.Header()
	maps.Copy(dstHeader, resp.Header)
	request.RemoveHopHeaders(dstHeader)

	policy.ExecuteMutations(w.Header(), &matchedRoute.Policies.Headers.Response)

	w.WriteHeader(resp.StatusCode)

	buf := copyBufferPool.Get().(*[]byte)
	defer copyBufferPool.Put(buf)

	_, _ = io.CopyBuffer(w, resp.Body, *buf)
}
