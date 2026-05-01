package admin

import (
	"io"
	"net/http"
)

type Probe interface {
	Liveness() error
}

type SystemHandler struct {
	probe Probe
}

func NewSystemHandler(probe Probe) *SystemHandler {
	return &SystemHandler{probe: probe}
}

func (h *SystemHandler) Livez(w http.ResponseWriter, _ *http.Request) {
	if err := h.probe.Liveness(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "service not alive")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok")
}
