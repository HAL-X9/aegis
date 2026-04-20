package http

import "net/http"

// NewMux registers HTTP routes for the public API and observability endpoints.
func NewMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", h.Livez)
	return mux
}
