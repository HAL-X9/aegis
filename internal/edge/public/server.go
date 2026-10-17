package public

import (
	"fmt"
	"net/http"

	"github.com/HAL-X9/aegis/internal/config"
	"github.com/HAL-X9/aegis/internal/dataplane/middleware"
	"github.com/HAL-X9/aegis/internal/edge/tlsconfig"
	"github.com/HAL-X9/aegis/internal/observe/metrics"
)

// NewPublicServer builds the public HTTP server and its request pipeline.
//
// collector is the HTTP metrics collector. It is required when
// cfg.Middleware.Metrics is enabled and ignored otherwise, so a caller that
// has the layer turned off need not create (and register) one.
func NewPublicServer(
	cfg *config.Runtime,
	executor RequestExecutor,
	collector *metrics.Metrics,
) (*http.Server, error) {
	tlsConfig, err := tlsconfig.Build(cfg.Listeners.Public.TLS)
	if err != nil {
		return nil, fmt.Errorf("build public TLS configuration: %w", err)
	}

	handler, err := buildHandler(cfg, NewRouter(NewForwardHandler(executor)), collector)
	if err != nil {
		return nil, err
	}

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

// buildHandler wraps next in the public middleware chain. From outermost to
// innermost the layers are timeout, metrics, request ID; the optional ones are
// left out when disabled in cfg.Middleware.
func buildHandler(cfg *config.Runtime, next http.Handler, collector *metrics.Metrics) (http.Handler, error) {
	handler := next

	if cfg.Middleware.RequestID {
		handler = middleware.RequestID(handler)
	}

	if cfg.Middleware.Metrics {
		if collector == nil {
			return nil, fmt.Errorf("metrics middleware is enabled but no metrics collector was provided")
		}
		handler = middleware.NewMetricsMiddleware(collector).Metrics(handler)
	}

	handler = middleware.NewTimeoutMiddleware(cfg.Listeners.Public.Timeouts.RequestTimeout).Timeout(handler)

	return handler, nil
}
