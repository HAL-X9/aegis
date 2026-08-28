package admin

import (
	"io"
	"net/http"
)

type Probe interface {
	Liveness() error
	Readiness() error
}

type SystemHandler struct {
	probe   Probe
	metrics http.Handler
}

func NewSystemHandler(probe Probe, metrics http.Handler) *SystemHandler {
	return &SystemHandler{
		probe:   probe,
		metrics: metrics,
	}
}

func (h *SystemHandler) Livez(w http.ResponseWriter, _ *http.Request) {
	if err := h.probe.Liveness(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "service not alive\n")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

func (h *SystemHandler) Readyz(w http.ResponseWriter, _ *http.Request) {
	if err := h.probe.Readiness(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "service not ready\n")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ready\n")
}

func (h *SystemHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	h.metrics.ServeHTTP(w, r)
}
