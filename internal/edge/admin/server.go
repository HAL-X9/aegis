package admin

import (
	"net/http"

	"github.com/HAL-X9/aegis/internal/config"
)

func NewSystemServer(
	cfg *config.Runtime,
	probe Probe,
	metrics http.Handler,
) *http.Server {
	systemHandler := NewSystemHandler(probe, metrics)
	systemRouter := NewRouter(systemHandler)

	return &http.Server{
		Addr:              cfg.Listeners.System.Addr,
		Handler:           systemRouter,
		ReadTimeout:       cfg.Listeners.System.Timeouts.ReadTimeout,
		ReadHeaderTimeout: cfg.Listeners.System.Timeouts.ReadHeaderTimeout,
		WriteTimeout:      cfg.Listeners.System.Timeouts.WriteTimeout,
		IdleTimeout:       cfg.Listeners.System.Timeouts.IdleTimeout,
		TLSConfig:         cfg.Listeners.System.TLS,
		MaxHeaderBytes:    cfg.Listeners.System.MaxHeaderBytes,
	}
}
