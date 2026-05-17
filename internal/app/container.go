package app

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/aegis/internal/config"
	"github.com/aegis/internal/controlplane/representation/source"
	"github.com/aegis/internal/dataplane/proxy"
	"github.com/aegis/internal/dataplane/router"
	edgeadmin "github.com/aegis/internal/edge/admin"
	edgepublic "github.com/aegis/internal/edge/public"
	"github.com/aegis/internal/observe/health"
)

type Dependencies struct {
	Config     *config.Runtime
	PublicHTTP *http.Server
	SystemHTTP *http.Server
	Health     *health.Health
	Engine     *router.Engine
}

func Bootstrap(cfg *config.Runtime, controlPlane *source.GatewayConfig) (*Dependencies, error) {
	if cfg == nil {
		return nil, fmt.Errorf("app config is nil")
	}
	if controlPlane == nil {
		return nil, fmt.Errorf("controlplane manifest is nil")
	}

	healthSvc := health.NewHealth()
	systemHTTP := edgeadmin.NewSystemServer(cfg, healthSvc)

	engine, err := router.BuildEngine(controlPlane)
	if err != nil {
		return nil, fmt.Errorf("pipeline route engine: %w", err)
	}

	upstreamTransport := newUpstreamTransport(&cfg.UpstreamTransport)
	executor := proxy.NewExecutor(engine, upstreamTransport)
	publicHTTP := edgepublic.NewPublicServer(cfg, executor)

	return &Dependencies{
		Config:     cfg,
		PublicHTTP: publicHTTP,
		SystemHTTP: systemHTTP,
		Health:     healthSvc,
		Engine:     engine,
	}, nil
}

func newUpstreamTransport(cfg *config.UpstreamTransport) *http.Transport {
	if cfg == nil {
		cfg = &config.UpstreamTransport{}
	}

	return &http.Transport{
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   cfg.DialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2: true,
	}
}
