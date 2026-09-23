package public

import (
	"fmt"
	"net/http"

	"github.com/HAL-X9/aegis/internal/config"
	"github.com/HAL-X9/aegis/internal/dataplane/middleware"
	"github.com/HAL-X9/aegis/internal/observe/metrics"
)

// NewPublicServer builds the public HTTP server and its request pipeline.
func NewPublicServer(
	cfg *config.Runtime,
	executor RequestExecutor,
	metrics *metrics.Metrics,
) (*http.Server, error) {
	tlsConfig, err := config.BuildTLSConfig(cfg.Listeners.Public.TLS)
	if err != nil {
		return nil, fmt.Errorf("build public TLS configuration: %w", err)
	}

	forward := NewForwardHandler(executor)
	publicHandler := NewRouter(forward)

	metricsMiddleware := middleware.NewMetricsMiddleware(metrics)
	timeoutMiddleware := middleware.NewTimeoutMiddleware(
		cfg.Listeners.Public.Timeouts.RequestTimeout,
	)

	var handler = publicHandler

	handler = middleware.RequestID(handler)
	handler = metricsMiddleware.Metrics(handler)
	handler = timeoutMiddleware.Timeout(handler)

	publicHTTP := &http.Server{
		Addr:              cfg.Listeners.Public.Addr,
		Handler:           handler,
		ReadTimeout:       cfg.Listeners.Public.Timeouts.ReadTimeout,
		ReadHeaderTimeout: cfg.Listeners.Public.Timeouts.ReadHeaderTimeout,
		WriteTimeout:      cfg.Listeners.Public.Timeouts.WriteTimeout,
		IdleTimeout:       cfg.Listeners.Public.Timeouts.IdleTimeout,
		TLSConfig:         tlsConfig,
		MaxHeaderBytes:    cfg.Listeners.Public.MaxHeaderBytes,
	}

	return publicHTTP, nil
}
