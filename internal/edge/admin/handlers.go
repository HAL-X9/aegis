package admin

import (
	"io"
	"net/http"
)

// Probe reports the process state exposed by the system listener.
type Probe interface {
	Liveness() error
	Readiness() error
}

// SystemHandler serves the operational endpoints of the system listener:
// liveness, readiness and Prometheus metrics.
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

// Livez answers 200 while the process can serve the probe, and 503 otherwise.
func (h *SystemHandler) Livez(w http.ResponseWriter, _ *http.Request) {
	if err := h.probe.Liveness(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "service not alive\n")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

// Readyz answers 200 while the process accepts traffic, and 503 once shutdown
// has begun.
func (h *SystemHandler) Readyz(w http.ResponseWriter, _ *http.Request) {
	if err := h.probe.Readiness(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "service not ready\n")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ready\n")
}

// Metrics serves the Prometheus exposition format through the handler given
// to NewSystemHandler.
func (h *SystemHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	h.metrics.ServeHTTP(w, r)
}
