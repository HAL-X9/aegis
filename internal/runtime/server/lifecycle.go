package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aegis/internal/runtime/config"
)

type Runtime struct {
	config *config.Runtime
	http   *http.Server
}

func NewRuntime(config *config.Runtime, http *http.Server) *Runtime {
	return &Runtime{config: config, http: http}
}

func (s *Runtime) Start(ctx context.Context) error {
	if s.http == nil {
		return fmt.Errorf("http server is nil")
	}

	listenErrCh := make(chan error, 1)
	go func() {
		err := s.http.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErrCh <- err
			return
		}
		listenErrCh <- nil
	}()

	select {
	case <-ctx.Done():
		return s.Shutdown()

	case err := <-listenErrCh:
		if err != nil {
			return fmt.Errorf("http server failed: %w", err)
		}
		return nil
	}
}

func (s *Runtime) Shutdown() error {
	if s.http == nil {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.http.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown failed: %w", err)
	}

	return nil
}
