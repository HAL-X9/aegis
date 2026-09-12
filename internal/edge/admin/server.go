package admin

import (
	"fmt"
	"net/http"

	"github.com/HAL-X9/aegis/internal/config"
)

func NewSystemServer(
	cfg *config.Runtime,
	probe Probe,
	metrics http.Handler,
) (*http.Server, error) {
	tlsConfig, err := config.BuildTLSConfig(cfg.Listeners.System.TLS)
	if err != nil {
		return nil, fmt.Errorf("build system TLS configuration: %w", err)
	}

	systemHandler := NewSystemHandler(probe, metrics)
	systemRouter := NewRouter(systemHandler)

	systemHTTP := &http.Server{
		Addr:              cfg.Listeners.System.Addr,
		Handler:           systemRouter,
		ReadTimeout:       cfg.Listeners.System.Timeouts.ReadTimeout,
		ReadHeaderTimeout: cfg.Listeners.System.Timeouts.ReadHeaderTimeout,
		WriteTimeout:      cfg.Listeners.System.Timeouts.WriteTimeout,
		IdleTimeout:       cfg.Listeners.System.Timeouts.IdleTimeout,
		TLSConfig:         tlsConfig,
		MaxHeaderBytes:    cfg.Listeners.System.MaxHeaderBytes,
	}
	return systemHTTP, nil
}
