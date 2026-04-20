package aegis

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const httpShutdownTimeout = 10 * time.Second

// httpServer wraps net/http.Server lifecycle (listen + graceful shutdown).
type httpServer struct {
	srv          *http.Server
	shutdownOnce sync.Once
}

func newHTTPServer(srv *http.Server) (*httpServer, error) {
	if srv == nil {
		return nil, fmt.Errorf("http server is nil")
	}
	return &httpServer{srv: srv}, nil
}

func (s *httpServer) Run(ctx context.Context) error {
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

func (s *httpServer) Close() error {
	return s.shutdown()
}

func (s *httpServer) shutdown() error {
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
