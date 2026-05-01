package aegis

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/aegis/internal/observe/health"
	"golang.org/x/sync/errgroup"
)

type serverGroup struct {
	public *httpServer
	system *httpServer
	health *health.Health

	closeOnce sync.Once
	closeErr  error
}

func newServerGroup(publicSrv, systemSrv *http.Server, health *health.Health) (*serverGroup, error) {
	pub, err := newHTTPServer(publicSrv)
	if err != nil {
		return nil, fmt.Errorf("servergroup.init public http server (%s): %w", publicSrv.Addr, err)
	}

	sys, err := newHTTPServer(systemSrv)
	if err != nil {
		// rollback partially initialized resource
		closeErr := pub.Close()
		if closeErr != nil {
			return nil, fmt.Errorf(
				"servergroup.init system http server (%s): %w (rollback public close failed: %v)",
				systemSrv.Addr, err, closeErr,
			)
		}
		return nil, fmt.Errorf("servergroup.init system http server (%s): %w", systemSrv.Addr, err)
	}

	return &serverGroup{public: pub, system: sys, health: health}, nil
}

func (g *serverGroup) Run(ctx context.Context) error {
	if g == nil {
		return fmt.Errorf("servergroup is nil")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	eg, egCtx := errgroup.WithContext(runCtx)

	runOne := func(name string, srv *httpServer) func() error {
		return func() error {
			if srv == nil {
				return nil
			}
			if err := srv.Run(egCtx); err != nil && !isExpectedRunErr(err) {
				return fmt.Errorf("%s server: %w", name, err)
			}
			return nil
		}
	}

	eg.Go(runOne("public", g.public))
	eg.Go(runOne("system", g.system))

	// Wait until both return or first fatal error appears.
	err := eg.Wait()
	if err == nil {
		return nil
	}

	// Make shutdown explicit and deterministic on fatal run failure.
	cancel()
	closeErr := g.Close()
	if closeErr != nil {
		return fmt.Errorf("servergroup.run: %w", errors.Join(err, fmt.Errorf("close after run failure: %w", closeErr)))
	}

	return fmt.Errorf("servergroup.run: %w", err)
}

func (g *serverGroup) Close() error {
	if g == nil {
		return nil
	}

	g.closeOnce.Do(func() {
		if g.health != nil {
			g.health.SetShuttingDown(true)
		}

		var err error

		// Stop public ingress first.
		if g.public != nil {
			if e := g.public.Close(); e != nil {
				err = errors.Join(err, fmt.Errorf("close public: %w", e))
			}
		}

		// Then stop system/admin endpoints.
		if g.system != nil {
			if e := g.system.Close(); e != nil {
				err = errors.Join(err, fmt.Errorf("close system: %w", e))
			}
		}

		if err != nil {
			g.closeErr = fmt.Errorf("servergroup.close: %w", err)
		}
	})

	return g.closeErr
}

func isExpectedRunErr(err error) bool {
	return err == nil || errors.Is(err, context.Canceled)
}
