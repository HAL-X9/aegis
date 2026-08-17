package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/HAL-X9/aegis/internal/observe/health"
	"golang.org/x/sync/errgroup"
)

const httpShutdownTimeout = 10 * time.Second

type runCloser interface {
	Name() string
	Run(ctx context.Context) error
	Close() error
}

type Lifecycle struct {
	public runCloser
	system runCloser
	health *health.Health

	closeOnce sync.Once
	closeErr  error
}

func NewLifecycle(public, system runCloser, h *health.Health) *Lifecycle {
	return &Lifecycle{
		public: public,
		system: system,
		health: h,
	}
}

func (l *Lifecycle) Run(ctx context.Context) error {
	if l == nil {
		return fmt.Errorf("lifecycle is nil")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	eg, egCtx := errgroup.WithContext(runCtx)
	runOne := func(c runCloser) func() error {
		return func() error {
			if c == nil {
				return nil
			}
			if err := c.Run(egCtx); err != nil && !isExpectedRunErr(err) {
				return fmt.Errorf("%s: %w", c.Name(), err)
			}
			return nil
		}
	}

	eg.Go(runOne(l.public))
	eg.Go(runOne(l.system))

	err := eg.Wait()
	if err == nil {
		return nil
	}

	cancel()
	if closeErr := l.Close(); closeErr != nil {
		return errors.Join(err, fmt.Errorf("close after run failure: %w", closeErr))
	}
	return err
}

func (l *Lifecycle) Close() error {
	if l == nil {
		return nil
	}

	l.closeOnce.Do(func() {
		if l.health != nil {
			l.health.SetShuttingDown(true)
		}

		var err error
		if l.public != nil {
			if e := l.public.Close(); e != nil {
				err = errors.Join(err, fmt.Errorf("close public: %w", e))
			}
		}
		if l.system != nil {
			if e := l.system.Close(); e != nil {
				err = errors.Join(err, fmt.Errorf("close system: %w", e))
			}
		}
		if err != nil {
			l.closeErr = fmt.Errorf("lifecycle.close: %w", err)
		}
	})

	return l.closeErr
}

func isExpectedRunErr(err error) bool {
	return err == nil || errors.Is(err, context.Canceled)
}

type HTTPServerComponent struct {
	name         string
	server       *http.Server
	shutdownOnce sync.Once
	shutdownErr  error
}

func NewHTTPServerComponent(name string, server *http.Server) (*HTTPServerComponent, error) {
	if server == nil {
		return nil, fmt.Errorf("http server is nil")
	}
	return &HTTPServerComponent{name: name, server: server}, nil
}

func (c *HTTPServerComponent) Name() string {
	return c.name
}

func (c *HTTPServerComponent) Run(ctx context.Context) error {
	listenErrCh := make(chan error, 1)
	go func() {
		log.SetFlags(log.LstdFlags | log.LUTC | log.Lmicroseconds)
		log.Printf("Aegis starting listener: %s", c.name)
		err := c.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErrCh <- err
			return
		}
		listenErrCh <- nil
	}()

	select {
	case <-ctx.Done():
		if err := c.Close(); err != nil {
			return fmt.Errorf("shutdown %s: %w", c.name, err)
		}
		return ctx.Err()
	case err := <-listenErrCh:
		if err != nil {
			return fmt.Errorf("listen %s: %w", c.name, err)
		}
		return nil
	}
}

func (c *HTTPServerComponent) Close() error {
	c.shutdownOnce.Do(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		defer cancel()
		if err := c.server.Shutdown(shutdownCtx); err != nil {
			c.shutdownErr = fmt.Errorf("http shutdown failed: %w", err)
		}
	})
	return c.shutdownErr
}
