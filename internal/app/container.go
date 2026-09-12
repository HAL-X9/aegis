package app

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/HAL-X9/aegis/internal/config"
	"github.com/HAL-X9/aegis/internal/controlplane/pipeline"
	"github.com/HAL-X9/aegis/internal/controlplane/schema"
	"github.com/HAL-X9/aegis/internal/dataplane/policy"
	"github.com/HAL-X9/aegis/internal/dataplane/proxy"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
	edgeadmin "github.com/HAL-X9/aegis/internal/edge/admin"
	edgepublic "github.com/HAL-X9/aegis/internal/edge/public"
	"github.com/HAL-X9/aegis/internal/observe/health"
	"github.com/HAL-X9/aegis/internal/observe/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Dependencies are everything App needs to run the gateway, built once at
// startup by Bootstrap.
type Dependencies struct {
	Config     *config.Runtime
	PublicHTTP *http.Server
	SystemHTTP *http.Server
	Health     *health.Health
	Engine     *router.Engine
}

// Bootstrap compiles the routes manifest, builds the routing engine and
// proxy executor, and constructs both HTTP servers. It performs no network
// I/O — the returned *http.Server values aren't listening on anything
// until something calls ListenAndServe on them (see Lifecycle.Run).
func Bootstrap(cfg *config.Runtime, manifest *schema.GatewayConfig) (*Dependencies, error) {
	if cfg == nil {
		return nil, fmt.Errorf("app config is nil")
	}
	if manifest == nil {
		return nil, fmt.Errorf("routes manifest is nil")
	}

	compiled, err := pipeline.Build(manifest)
	if err != nil {
		return nil, fmt.Errorf("build gateway pipeline: %w", err)
	}

	engine, err := router.BuildEngine(compiled)
	if err != nil {
		return nil, fmt.Errorf("build route engine: %w", err)
	}

	rateLimiters := policy.NewRateLimiterSet(compiled.Policies.RateLimits)
	upstreamTransport := newUpstreamTransport(&cfg.UpstreamTransport)
	executor := proxy.NewExecutor(engine, rateLimiters, upstreamTransport)

	metricsCollector := metrics.NewMetrics(prometheus.DefaultRegisterer)
	healthSvc := health.NewHealth()

	systemHTTP, err := edgeadmin.NewSystemServer(cfg, healthSvc, promhttp.Handler())
	if err != nil {
		return nil, fmt.Errorf("build system HTTP server: %w", err)
	}
	publicHTTP, err := edgepublic.NewPublicServer(cfg, executor, metricsCollector)
	if err != nil {
		return nil, fmt.Errorf("build public HTTP server: %w", err)
	}

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
