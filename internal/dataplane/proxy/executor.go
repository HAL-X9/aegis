package proxy

import (
	"io"
	"net/http"

	"github.com/aegis/internal/contracts/methodmask"
	"github.com/aegis/internal/dataplane/router"
)

type Executor struct {
	engine    *router.Engine
	transport http.RoundTripper
}

func NewExecutor(engine *router.Engine, transport http.RoundTripper) *Executor {
	return &Executor{engine: engine, transport: transport}
}

// Executor is an HTTP handler responsible for resolving incoming requests
// against a routing engine and delegating them to the appropriate upstream
// via a configured transport.
//
// It encapsulates:
//   - engine: a routing engine used for path-based lookup and route resolution.
//   - transport: an HTTP RoundTripper used to execute outbound requests.
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
			originURL := candidate.Route.Upstream
			path := r.URL.EscapedPath()
			target = originURL + path
			break
		}
	}
	if target == "" {
		http.Error(w, "unsupported HTTP method", http.StatusMethodNotAllowed)
		return
	}

	request, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, "failed to build HTTP request", http.StatusInternalServerError)
		return
	}

	response, err := executor.transport.RoundTrip(request)
	if err != nil {
		http.Error(w, "Bad Gateway: upstream service request failed", http.StatusBadGateway)
		return
	}
	defer func() {
		if err = response.Body.Close(); err != nil {
			return
		}
	}()

	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}
