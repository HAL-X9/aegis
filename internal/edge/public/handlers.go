package public

import "net/http"

// RequestExecutor is the dataplane execution entrypoint used by the public edge.
type RequestExecutor interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

// ForwardHandler forwards public HTTP traffic to the dataplane executor.
type ForwardHandler struct {
	executor RequestExecutor
}

// NewForwardHandler creates a public edge handler backed by the dataplane executor.
func NewForwardHandler(executor RequestExecutor) *ForwardHandler {
	return &ForwardHandler{
		executor: executor,
	}
}

func (h *ForwardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.executor.ServeHTTP(w, r)
}
