package proxy

import (
	"net/http"

	"github.com/aegis/internal/dataplane/router"
)

type Executor struct {
	engine *router.Engine
}

func NewExecutor(engine *router.Engine) *Executor {
	return &Executor{engine: engine}
}

func (executor *Executor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if executor.engine == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	// routes := executor.engine.Lookup([]byte(r.URL.Path))
}
