package public

import "net/http"

// RequestExecutor is the dataplane execution entrypoint used by public edge handlers.
type RequestExecutor interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

// ForwardHandler is a thin HTTP adapter that delegates user traffic to dataplane executor.
type ForwardHandler struct {
	executor RequestExecutor
}

func NewForwardHandler(executor RequestExecutor) *ForwardHandler {
	return &ForwardHandler{executor: executor}
}

func (h *ForwardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.executor.ServeHTTP(w, r)
}
