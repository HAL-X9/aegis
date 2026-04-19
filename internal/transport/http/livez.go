package http

import (
	"io"
	"net/http"
)

type LivenessChecker interface {
	Liveness() error
}

type Handler struct {
	health LivenessChecker
}

func NewHandler(health LivenessChecker) *Handler {
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
