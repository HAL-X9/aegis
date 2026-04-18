package http

import (
	"io"
	"net/http"

	"github.com/aegis/internal/ports"
)

type Handler struct {
	health ports.HealthService
}

func NewHandler(health ports.HealthService) *Handler {
	return &Handler{health: health}
}

func (h *Handler) Livez(w http.ResponseWriter, r *http.Request) {
	if err := h.health.Liveness(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "service not alive")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok")
}
