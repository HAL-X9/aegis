package health

import (
	"errors"
	"sync/atomic"
)

// Health tracks process liveness and readiness for the system listener's
// probes. Its zero value, as returned by NewHealth, is alive and ready. It is
// safe for concurrent use.
type Health struct {
	shuttingDown atomic.Bool
}

func NewHealth() *Health {
	return &Health{}
}

// SetShuttingDown marks the process as shutting down, which makes Readiness
// fail. Lifecycle sets it before the listeners are closed.
func (h *Health) SetShuttingDown(v bool) {
	h.shuttingDown.Store(v)
}

// Liveness always succeeds: the process is alive if it can answer at all.
func (h *Health) Liveness() error {
	return nil
}

// Readiness fails once shutdown has begun.
func (h *Health) Readiness() error {
	if h.shuttingDown.Load() {
		return errors.New("service shutting down")
	}
	return nil
}
