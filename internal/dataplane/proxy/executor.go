package proxy

import (
	"io"
	"net/http"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
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

	// transport is responsible for executing outbound HTTP requests.
	// It must be non-nil and safe for concurrent use.
	transport http.RoundTripper
}

// NewExecutor creates a new Executor instance.
// Both engine and transport are required for correct operation.
func NewExecutor(engine *router.Engine, transport http.RoundTripper) *Executor {
	return &Executor{
		engine:    engine,
		transport: transport,
	}
}

// ServeHTTP resolves the incoming request using the routing engine and
// proxies it to a matching upstream service.
//
// Behavior:
//   - returns 503 if the routing engine is unavailable
//   - returns 404 if no route matches the request path
//   - returns 405 if no route supports the request method
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

	// Resolve all route candidates matching the incoming request path.
	candidates := executor.engine.Lookup(r.URL.Path)
	if len(candidates) == 0 {
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

	var target string
	var methodMatch bool
	var matchedEntry *router.RouteIndexEntry

	// Select the first route candidate that satisfies both method and header
	// predicates. Build the upstream target URL from the matched route upstream
	// origin and the escaped request path.
	for _, candidate := range candidates {
		if candidate.Route.Match.Methods&methodBit != 0 {
			methodMatch = true
			if router.HeadersMatch(candidate.Route.Match.Headers, r.Header) {
				target = candidate.Upstream + r.URL.EscapedPath()
				if r.URL.RawQuery != "" {
					target += "?" + r.URL.RawQuery
				}
				matchedEntry = candidate
				break
			}
			continue
		}
	}

	// At least one route matched the path, but none accepted the method.
	if target == "" {
		if !methodMatch {
			http.Error(w, "method not allowed for matched route", http.StatusMethodNotAllowed)
			return
		}
		http.NotFound(w, r)
		return
	}

	// Construct the upstream request while preserving:
	//   - request context
	//   - HTTP method
	//   - original request body stream
	req, err := http.NewRequestWithContext(
		r.Context(),
		r.Method,
		target,
		r.Body,
	)
	if err != nil {
		http.Error(w, "failed to create upstream request", http.StatusInternalServerError)
		return
	}

	req.Header = r.Header.Clone()
	if matchedEntry != nil {
		policy.ExecuteMutations(req.Header, &matchedEntry.Route.Headers.Request)
	}

	request.RemoveHopHeaders(req.Header)

	// Execute the upstream request using the configured transport.
	resp, err := executor.transport.RoundTrip(req)
	if err != nil {
		http.Error(w, "bad gateway: upstream request failed", http.StatusBadGateway)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	if matchedEntry != nil {
		policy.ExecuteMutations(w.Header(), &matchedEntry.Route.Headers.Response)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
