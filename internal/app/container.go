package app

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/HAL-X9/aegis/internal/config"
	"github.com/HAL-X9/aegis/internal/controlplane/pipeline"
	"github.com/HAL-X9/aegis/internal/controlplane/validate"
	"github.com/HAL-X9/aegis/internal/dataplane/policy"
	"github.com/HAL-X9/aegis/internal/dataplane/proxy"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
	edgeadmin "github.com/HAL-X9/aegis/internal/edge/admin"
	edgepublic "github.com/HAL-X9/aegis/internal/edge/public"
	"github.com/HAL-X9/aegis/internal/edge/tlsconfig"
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
func Bootstrap(cfg *config.Runtime, manifest validate.Validated) (*Dependencies, error) {
	if cfg == nil {
		return nil, fmt.Errorf("app config is nil")
	}

	compiled, err := pipeline.Build(manifest)
	if err != nil {
		return nil, fmt.Errorf("build gateway pipeline: %w", err)
	}

	engine, err := router.NewEngine(compiled)
	if err != nil {
		return nil, fmt.Errorf("build route engine: %w", err)
	}

	rateLimiters := policy.NewRateLimiterSet(compiled.RateLimits.Items)

	upstreamTransport := newUpstreamTransport(&cfg.UpstreamTransport)

	executor, err := proxy.NewExecutor(upstreamTransport)
	if err != nil {
		return nil, fmt.Errorf("build proxy executor: %w", err)
	}

	executor.Publish(&proxy.View{
		Engine:   engine,
		Limiters: rateLimiters,
	})

	// The HTTP metrics collector exists only when its middleware is enabled,
	// so a disabled gateway registers no aegis_http_* series. The system
	// listener's /metrics endpoint is independent of this switch.
	var metricsCollector *metrics.Metrics
	if cfg.Middleware.Metrics {
		metricsCollector = metrics.NewMetrics(prometheus.DefaultRegisterer)
	}

	healthSvc := health.NewHealth()

	systemHandler := edgeadmin.NewRouter(edgeadmin.NewSystemHandler(healthSvc, promhttp.Handler()))
	systemHTTP, err := newHTTPServer(cfg.Listeners.System, systemHandler)
	if err != nil {
		return nil, fmt.Errorf("build system HTTP server: %w", err)
	}

	publicHandler := edgepublic.NewHandler(executor, edgepublic.Options{
		RequestTimeout: cfg.Listeners.Public.Timeouts.RequestTimeout,
		RequestID:      cfg.Middleware.RequestID,
		Metrics:        metricsCollector,
	})
	publicHTTP, err := newHTTPServer(cfg.Listeners.Public, publicHandler)
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

// newHTTPServer builds the net/http server of one listener around handler,
// loading its TLS certificate when the listener has a tls section. It does not
// open the socket; see Lifecycle.Run.
func newHTTPServer(listener config.HTTP, handler http.Handler) (*http.Server, error) {
	var tlsConfig *tls.Config
	if listener.TLS != nil {
		var err error
		tlsConfig, err = tlsconfig.Load(listener.TLS.CertFile, listener.TLS.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("build TLS configuration: %w", err)
		}
	}

	return &http.Server{
		Addr:              listener.Addr,
		Handler:           handler,
		ReadTimeout:       listener.Timeouts.ReadTimeout,
		ReadHeaderTimeout: listener.Timeouts.ReadHeaderTimeout,
		WriteTimeout:      listener.Timeouts.WriteTimeout,
		IdleTimeout:       listener.Timeouts.IdleTimeout,
		TLSConfig:         tlsConfig,
		MaxHeaderBytes:    listener.MaxHeaderBytes,
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
		DisableCompression: true,
		ForceAttemptHTTP2:  true,
	}
}
