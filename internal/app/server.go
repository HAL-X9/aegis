package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const httpShutdownTimeout = 10 * time.Second

// HTTPServer wraps net/http.Server lifecycle (listen + graceful shutdown).
type HTTPServer struct {
	srv          *http.Server
	shutdownOnce sync.Once
}

func NewHTTPServer(srv *http.Server) (*HTTPServer, error) {
	if srv == nil {
		return nil, fmt.Errorf("http server is nil")
	}
	return &HTTPServer{srv: srv}, nil
}

func (s *HTTPServer) Run(ctx context.Context) error {
	if s.srv == nil {
		return fmt.Errorf("http server is nil")
	}

	listenErrCh := make(chan error, 1)
	go func() {
		err := s.srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErrCh <- err
			return
		}
		listenErrCh <- nil
	}()

	select {
	case <-ctx.Done():
		if err := s.shutdown(); err != nil {
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

func (s *HTTPServer) Close() error {
	return s.shutdown()
}

func (s *HTTPServer) shutdown() error {
	var outErr error
	s.shutdownOnce.Do(func() {
		if s.srv == nil {
			return
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer cancel()
		if err := s.srv.Shutdown(shutdownCtx); err != nil {
			outErr = fmt.Errorf("http shutdown failed: %w", err)
		}
	})
	return outErr
}
