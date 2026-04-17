package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func (a *Application) Run(ctx context.Context) error {
	if a.http == nil {
		return fmt.Errorf("http server is nil")
	}

	listenErrCh := make(chan error, 1)
	go func() {
		err := a.http.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErrCh <- err
			return
		}
		listenErrCh <- nil
	}()

	select {
	case <-ctx.Done():
		if err := a.shutdownHTTP(); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		return ctx.Err()

	case err := <-listenErrCh:
		if err != nil {
			return fmt.Errorf("http server failed: %w", err)
		}
		return nil
	}
}

func (a *Application) Close() error {
	return a.shutdownHTTP()
}

func (a *Application) shutdownHTTP() error {
	var outErr error
	a.shutdownOnce.Do(func() {
		if a.http == nil {
			return
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.http.Shutdown(shutdownCtx); err != nil {
			outErr = fmt.Errorf("http shutdown failed: %w", err)
		}
	})
	return outErr
}
