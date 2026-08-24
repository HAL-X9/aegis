package admin

import (
	"net/http"
)

// NewRouter registers HTTP routes for the public API and observability endpoints.
func NewRouter(h *SystemHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", h.Livez)
	mux.HandleFunc("GET /readyz", h.Readyz)
	return mux
}
