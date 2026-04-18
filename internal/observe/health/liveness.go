package health

import (
	"errors"
	"sync/atomic"
)

type Health struct {
	shuttingDown atomic.Bool
}

func NewHealth() *Health {
	return &Health{}
}

func (h *Health) SetShuttingDown(v bool) {
	h.shuttingDown.Store(v)
}

func (h *Health) Liveness() error {
	if h.shuttingDown.Load() {
		return errors.New("service shutting down")
	}
	return nil
}
