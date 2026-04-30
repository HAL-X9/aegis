package proxy

import (
	"net/http"

	"github.com/aegis/internal/dataplane/router"
)

type Executor struct {
	engine    *router.Engine
	transport http.RoundTripper
}

func NewExecutor(engine *router.Engine, transport http.RoundTripper) *Executor {
	return &Executor{engine: engine, transport: transport}
}

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

	// route := candidates[0].Route
}
