package proxy

import (
	"io"
	"net/http"

	"github.com/aegis/internal/contracts/methodmask"
	"github.com/aegis/internal/dataplane/router"
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
	if executor.engine == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	candidates := executor.engine.Lookup([]byte(r.URL.Path))
	if len(candidates) == 0 {
		http.NotFound(w, r)
		return
	}

	methodBit, ok := methodmask.MethodBit(r.Method)
	if !ok {
		http.Error(w, "unsupported HTTP method", http.StatusMethodNotAllowed)
		return
	}

	var target string

	for _, candidate := range candidates {
		if candidate.Route.Match.Methods&methodBit != 0 {
			target = candidate.Route.Upstream + r.URL.EscapedPath()
			break
		}
	}

	if target == "" {
		http.Error(w, "method not allowed for matched route", http.StatusMethodNotAllowed)
		return
	}

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

	resp, err := executor.transport.RoundTrip(req)
	if err != nil {
		http.Error(w, "bad gateway: upstream request failed", http.StatusBadGateway)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
