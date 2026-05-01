package aegis

import (
	"fmt"
	"net/http"

	"github.com/aegis/internal/admin"
	"github.com/aegis/internal/config"
	"github.com/aegis/internal/controlplane/model"
	"github.com/aegis/internal/dataplane/proxy"
	"github.com/aegis/internal/dataplane/router"
	"github.com/aegis/internal/observe/health"
)

// Dependencies is the fully wired object graph for the process.
// It contains only long-lived components (no ephemeral state).
type Dependencies struct {
	Config     *config.Runtime
	PublicHTTP *http.Server
	SystemHTTP *http.Server
	Health     *health.Health
	Engine     *router.Engine
}

// Bootstrap wires configuration into concrete implementations.
// It does NOT start any listeners.
func Bootstrap(cfg *config.Runtime, controlPlane *model.GatewayConfig) (*Dependencies, error) {

	// ---- Validate input -----------------------------------------------------

	if cfg == nil {
		return nil, fmt.Errorf("app config is nil")
	}
	if controlPlane == nil {
		return nil, fmt.Errorf("controlplane manifest is nil")
	}

	// ---- Core services ------------------------------------------------------

	healthSvc := health.NewHealth()

	// ---- System plane (health, admin endpoints) -----------------------------

	systemHandler := admin.NewSystemHandler(healthSvc).Handler()

	systemHTTP := &http.Server{
		Addr:              cfg.Listeners.System.Addr,
		Handler:           systemHandler,
		ReadTimeout:       cfg.Listeners.System.Timeouts.ReadTimeout,
		ReadHeaderTimeout: cfg.Listeners.System.Timeouts.ReadHeaderTimeout,
		WriteTimeout:      cfg.Listeners.System.Timeouts.WriteTimeout,
		IdleTimeout:       cfg.Listeners.System.Timeouts.IdleTimeout,
		TLSConfig:         cfg.Listeners.System.TLS,
		MaxHeaderBytes:    cfg.Listeners.System.MaxHeaderBytes,
	}

	// ---- Dataplane (routing + execution) ------------------------------------

	engine, err := router.BuildEngine(controlPlane)
	if err != nil {
		return nil, fmt.Errorf("build route engine: %w", err)
	}

	upstreamTransport := newUpstreamTransport(&cfg.UpstreamTransport)
	executor := proxy.NewExecutor(engine, upstreamTransport)

	// ---- Public plane (user traffic) ----------------------------------------

	publicHandler := http.HandlerFunc(executor.ServeHTTP)

	publicHTTP := &http.Server{
		Addr:              cfg.Listeners.Public.Addr,
		Handler:           publicHandler,
		ReadTimeout:       cfg.Listeners.Public.Timeouts.ReadTimeout,
		ReadHeaderTimeout: cfg.Listeners.Public.Timeouts.ReadHeaderTimeout,
		WriteTimeout:      cfg.Listeners.Public.Timeouts.WriteTimeout,
		IdleTimeout:       cfg.Listeners.Public.Timeouts.IdleTimeout,
		TLSConfig:         cfg.Listeners.Public.TLS,
		MaxHeaderBytes:    cfg.Listeners.Public.MaxHeaderBytes,
	}

	// ---- Assemble -----------------------------------------------------------

	return &Dependencies{
		Config:     cfg,
		PublicHTTP: publicHTTP,
		SystemHTTP: systemHTTP,
		Health:     healthSvc,
		Engine:     engine,
	}, nil
}
