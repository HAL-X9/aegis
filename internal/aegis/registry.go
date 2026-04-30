package aegis

import (
	"fmt"
	"net/http"

	"github.com/aegis/internal/config"
	"github.com/aegis/internal/controlplane/model"
	"github.com/aegis/internal/dataplane/proxy"
	"github.com/aegis/internal/dataplane/router"
	"github.com/aegis/internal/observe/health"
)

// Dependencies is the constructed object graph for this process (explicit, no container).
type Dependencies struct {
	Runtime *config.Runtime
	Health  *health.Health
	HTTP    *http.Server
	Engine  *router.Engine
}

// Bootstrap wires configuration into concrete implementations. It does not start listeners.
func Bootstrap(cfg *config.Runtime, controlPlane *model.GatewayConfig) (*Dependencies, error) {
	if cfg == nil {
		return nil, fmt.Errorf("app config is nil")
	}
	if controlPlane == nil {
		return nil, fmt.Errorf("controlplane manifest is nil")
	}

	// Core process services (health, outbound transport).
	hsvc := health.NewHealth()
	transport := newUpstreamTransport(&cfg.UpstreamTransport)

	// Build dataplane routing structures from validated control-plane config.
	engine, err := router.BuildEngine(controlPlane)
	if err != nil {
		return nil, fmt.Errorf("failed to build route engine: %w", err)
	}

	// HTTP request execution pipeline.
	executor := proxy.NewExecutor(engine, transport)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executor.ServeHTTP(w, r)
	})

	// Public HTTP server with app-configured limits and timeouts.
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
