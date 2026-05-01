package app

import (
	"fmt"
	"net/http"

	"github.com/aegis/internal/config"
	"github.com/aegis/internal/controlplane/model"
	"github.com/aegis/internal/dataplane/proxy"
	"github.com/aegis/internal/dataplane/router"
	edgeadmin "github.com/aegis/internal/edge/admin"
	edgepublic "github.com/aegis/internal/edge/public"
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

	systemHTTP := edgeadmin.NewSystemServer(cfg, healthSvc)

	// ---- Dataplane (routing + execution) ------------------------------------

	engine, err := router.BuildEngine(controlPlane)
	if err != nil {
		return nil, fmt.Errorf("build route engine: %w", err)
	}

	upstreamTransport := newUpstreamTransport(&cfg.UpstreamTransport)
	executor := proxy.NewExecutor(engine, upstreamTransport)

	// ---- Public plane (user traffic) ----------------------------------------

	publicHTTP := edgepublic.NewPublicServer(cfg, executor)

	// ---- Assemble -----------------------------------------------------------

	return &Dependencies{
		Config:     cfg,
		PublicHTTP: publicHTTP,
		SystemHTTP: systemHTTP,
		Health:     healthSvc,
		Engine:     engine,
	}, nil
}
