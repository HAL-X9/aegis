package app

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Runtime struct {
	lifecycle *Lifecycle
	shutdown  *ShutdownManager
}

func NewRuntime(lifecycle *Lifecycle, timeout time.Duration) *Runtime {
	return &Runtime{
		lifecycle: lifecycle,
		shutdown:  NewShutdownManager(timeout),
	}
}

func (r *Runtime) Run(ctx context.Context) error {
	if r == nil || r.lifecycle == nil {
		return fmt.Errorf("runtime is not initialized")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- r.lifecycle.Run(runCtx)
	}()

	select {
	case err := <-runErrCh:
		return err
	case <-r.shutdown.Wait(runCtx):
		shutdownErr := r.Close()
		cancel()
		runErr := <-runErrCh
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			if shutdownErr != nil {
				return fmt.Errorf("run failed during shutdown: %w", errors.Join(runErr, shutdownErr))
			}
			return runErr
		}
		return shutdownErr
	}
}

func (r *Runtime) Close() error {
	if r == nil || r.lifecycle == nil {
		return nil
	}
	return r.lifecycle.Close()
}
