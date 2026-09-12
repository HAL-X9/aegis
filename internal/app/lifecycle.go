package app

import (
	"context"
	"fmt"
	"time"

	"github.com/HAL-X9/aegis/internal/observe/health"
	"golang.org/x/sync/errgroup"
)

// shutdownTimeout bounds how long stop waits for in-flight requests to
// drain before it force-closes both listeners.
//
// config.Runtime has no Shutdown.Timeout field (or equivalent) today, so
// this is a plain constant rather than a config-driven value with a
// fallback — a config field that's silently ignored whenever it's set is
// its own kind of implicit behavior. If a future need arises to configure
// this per-deployment, add a field to config.Runtime and thread it through
// NewLifecycle's parameters explicitly, rather than reaching back into
// Dependencies.Config for it here.
const shutdownTimeout = 15 * time.Second

// Lifecycle owns starting and stopping both HTTP listeners as a single
// unit.
//
// There is exactly one condition that can trigger a stop — the context
// passed to Run being canceled, or either listener returning an error —
// and exactly one place that acts on it (stop, below). That is what rules
// out two independent shutdown sequences racing, or a listener being
// closed twice: not a lock, but the fact that only one code path ever
// calls stop, and it calls it once.
type Lifecycle struct {
	public *httpComponent
	system *httpComponent
	health *health.Health
}

// NewLifecycle builds a Lifecycle from bootstrapped dependencies.
func NewLifecycle(deps *Dependencies) (*Lifecycle, error) {
	if deps == nil {
		return nil, fmt.Errorf("dependencies is nil")
	}

	public, err := newHTTPComponent("public", deps.PublicHTTP)
	if err != nil {
		return nil, err
	}
	system, err := newHTTPComponent("system", deps.SystemHTTP)
	if err != nil {
		return nil, err
	}

	return &Lifecycle{
		public: public,
		system: system,
		health: deps.Health,
	}, nil
}

// Run starts both listeners, blocks until ctx is canceled or either
// listener fails, then stops both before returning.
//
// egCtx becomes Done for exactly one of two reasons: the caller canceled
// ctx (the normal, signal-driven shutdown path), or errgroup canceled it
// itself because public.run or system.run returned a non-nil error (a
// listener failure). Either way, the same single goroutine below performs
// the stop. On the graceful path every goroutine here returns nil, so
// Run itself returns nil — callers don't need to special-case
// context.Canceled.
func (l *Lifecycle) Run(ctx context.Context) error {
	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(l.public.run)
	eg.Go(l.system.run)
	eg.Go(func() error {
		<-egCtx.Done()
		return l.stop()
	})

	return eg.Wait()
}

// stop marks the process not-ready, then closes both listeners in
// parallel, each bounded by shutdownTimeout.
//
// It is reachable from exactly one call site (the goroutine in Run above),
// which itself runs at most once per Run call — so stop needs no guard
// against being invoked twice.
func (l *Lifecycle) stop() error {
	if l.health != nil {
		l.health.SetShuttingDown(true)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	var eg errgroup.Group
	eg.Go(func() error { return l.public.shutdown(shutdownCtx) })
	eg.Go(func() error { return l.system.shutdown(shutdownCtx) })
	return eg.Wait()
}
