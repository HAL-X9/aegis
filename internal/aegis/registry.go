package aegis

import (
	"fmt"
	"net/http"

	"github.com/aegis/internal/config/controlplane"
	runtimeconfig "github.com/aegis/internal/config/runtime"
	"github.com/aegis/internal/dataplane/proxy"
	"github.com/aegis/internal/dataplane/router"
	"github.com/aegis/internal/observe/health"
)

// Dependencies is the constructed object graph for this process (explicit, no container).
type Dependencies struct {
	Runtime *runtimeconfig.Runtime
	Health  *health.Health
	HTTP    *http.Server
	Engine  *router.Engine
}

// Bootstrap wires configuration into concrete implementations. It does not start listeners.
func Bootstrap(cfg *runtimeconfig.Runtime, controlPlane *controlplane.AegisManifest) (*Dependencies, error) {
	if cfg == nil {
		return nil, fmt.Errorf("runtime config is nil")
	}
	if controlPlane == nil {
		return nil, fmt.Errorf("controlplane manifest is nil")
	}

	hsvc := health.NewHealth()
	// h := httptransport.NewHandler(hsvc)
	transport := newUpstreamTransport(&cfg.UpstreamTransport)

	engine, err := router.BuildEngine(controlPlane)
	if err != nil {
		return nil, fmt.Errorf("failed to build route engine: %w", err)
	}

	executor := proxy.NewExecutor(engine, transport)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executor.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           handler,
		ReadTimeout:       cfg.HTTP.Timeouts.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.Timeouts.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.Timeouts.WriteTimeout,
		IdleTimeout:       cfg.HTTP.Timeouts.IdleTimeout,
		TLSConfig:         cfg.HTTP.TLS,
		MaxHeaderBytes:    cfg.HTTP.MaxHeaderBytes,
	}

	return &Dependencies{
		Runtime: cfg,
		Health:  hsvc,
		HTTP:    srv,
	}, nil
}
