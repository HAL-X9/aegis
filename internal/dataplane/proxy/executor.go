package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/HAL-X9/aegis/internal/dataplane/fault"
	"github.com/HAL-X9/aegis/internal/dataplane/policy"
	"github.com/HAL-X9/aegis/internal/dataplane/request"
	"github.com/HAL-X9/aegis/internal/dataplane/retry"
	"github.com/HAL-X9/aegis/internal/dataplane/routelabel"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
	"github.com/HAL-X9/aegis/internal/httpmethod"
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

	// observer receives every error response; never nil.
	observer Observer
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
// checked once here instead of on every request. observer receives every
// error response the Executor produces; nil discards them.
//
// The returned Executor serves 503 for every request until Publish is
// called at least once; call Publish with the initial View before handing
// the Executor to a server.
func NewExecutor(transport http.RoundTripper, observer Observer) (*Executor, error) {
	if transport == nil {
		return nil, fmt.Errorf("proxy: transport must not be nil")
	}
	if observer == nil {
		observer = discardObserver{}
	}
	return &Executor{transport: transport, observer: observer}, nil
}

// Publish atomically swaps in a new View. Callers build the complete View
// (Engine + Limiters) before calling Publish — there is no partial or
// in-place update, so a request mid-flight always sees either the whole
// old View or the whole new one, never a mix.
func (executor *Executor) Publish(v *View) {
	executor.view.Store(v)
}

// newUpstreamRequest returns a shallow copy of r with context ctx, addressed
// to target, with the canonical path, r's query, and a header built by
// request.UpstreamHeader. It keeps r's body, sets Host to the target host,
// and clears RequestURI, which client requests must not set.
func newUpstreamRequest(ctx context.Context, r *http.Request, target *url.URL, path string) *http.Request {
	out := &upstreamRequest{url: *target}
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

// retarget returns a shallow copy of req addressed to target. A repeated try
// gets its own request instead of modifying req, because the transport may
// still be reading req after the try that sent it has failed.
func retarget(req *http.Request, target *url.URL) *http.Request {
	out := &upstreamRequest{req: *req, url: *req.URL}
	out.url.Scheme = target.Scheme
	out.url.Host = target.Host
	out.req.URL = &out.url
	out.req.Host = target.Host
	return &out.req
}

// ServeHTTP resolves the incoming request using the routing engine and
// proxies it to a matching upstream service.
//
// Outcomes, in the order they are decided (the contract is
// docs/routing-and-errors.md §2; change both together). Each failure is
// answered by fail with the problem document of its reason:
//   - NotReady (503) if no View has been published yet
//   - InvalidPath (400) if the request path has no canonical form (see
//     request.CanonicalPath); otherwise the canonical path is both routed
//     and forwarded (invariant I-001)
//   - UnsupportedMethod (405) if the request method is not a supported method
//   - NoRoute (404) if no route matches the path, or if some candidate allows
//     the method but none satisfies its header predicates
//   - MethodNotAllowed (405) if candidates match the path but none allows the
//     method
//   - RateLimited (429) if the matched route's rate-limit policy rejects the
//     client, identified by the address of the connection's peer
//   - UpstreamTimeout (504) if the upstream exchange times out (see isTimeout)
//   - UpstreamFailed (502) if the upstream exchange fails for any other reason
//   - otherwise the upstream response is passed through
//
// The service's targets are taken in turn (see internal/dataplane/balance).
// Idempotent, body-less requests are retried on transport errors and on
// 502/503/504 when the route's service configures more than one attempt (see
// internal/dataplane/retry); each retry goes to the next target. The rate
// limiter is consulted once, before the first attempt. The request body is
// forwarded as-is to the upstream service.
func (executor *Executor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	view := executor.view.Load()
	if view == nil {
		executor.fail(w, r, Failure{Reason: fault.NotReady})
		return
	}

	path, ok := request.CanonicalPath(r.URL)
	if !ok {
		executor.fail(w, r, Failure{Reason: fault.InvalidPath})
		return
	}

	// Resolve the bitmask representation of the incoming HTTP method.
	// Unsupported methods are rejected before any lookup. RFC 9110 would
	// suggest 501 here; docs/routing-and-errors.md §3 records the open item.
	methodBit, ok := httpmethod.Bit(r.Method)
	if !ok {
		executor.fail(w, r, Failure{Reason: fault.UnsupportedMethod})
		return
	}

	matchedRoute, outcome := view.Engine.Resolve(path, methodBit, r.Header)
	switch outcome {
	case router.NotFound:
		executor.fail(w, r, Failure{Reason: fault.NoRoute})
		return
	case router.MethodNotAllowed:
		executor.fail(w, r, Failure{Reason: fault.MethodNotAllowed})
		return
	}

	routeLabel := matchedRoute.Name
	if routeLabel == "" {
		routeLabel = "unnamed-route"
	}
	routelabel.FromContext(r.Context()).SetRoutePattern(routeLabel)

	// A RemoteAddr that is not an address and port leaves the zero Addr, which
	// RateLimiterSet.Allow treats as one shared client.
	client, _ := netip.ParseAddrPort(r.RemoteAddr)
	if !view.Limiters.Allow(matchedRoute, client.Addr()) {
		executor.fail(w, r, Failure{Reason: fault.RateLimited, Route: matchedRoute.Name})
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
	targets := view.Balancer.Pick(matchedRoute.Service)
	req := newUpstreamRequest(ctx, r, targets.Target(0), path)
	headerNames := view.Engine.HeaderNames()

	policy.ExecuteMutations(req.Header, &matchedRoute.Policies.Headers.Request, headerNames)

	// The client's hop-by-hop headers are already gone (see
	// request.UpstreamHeader); this removes those a policy introduced, so a
	// client's Connection tokens never reach the gateway's own headers.
	request.RemoveHopHeaders(req.Header)

	// A repeated try goes to the next target in the selection, carrying the
	// request as prepared above. With a single target every try reuses req.
	var reroute retry.Reroute
	if targets.Len() > 1 {
		reroute = func(try int) *http.Request {
			return retarget(req, targets.Target(try-1))
		}
	}

	resp, err := retry.Do(executor.transport, req, matchedRoute.Policies.Retry, reroute)
	if err != nil {
		reason := fault.UpstreamFailed
		if isTimeout(err) {
			reason = fault.UpstreamTimeout
		}
		executor.fail(w, r, Failure{Reason: reason, Route: matchedRoute.Name, Cause: err})
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
