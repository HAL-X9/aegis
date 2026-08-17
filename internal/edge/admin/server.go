package admin

import (
	"net/http"

	"github.com/HAL-X9/aegis/internal/config"
)

func NewSystemServer(cfg *config.Runtime, probe Probe) *http.Server {
	h := NewSystemHandler(probe)
	systemHandler := NewRouter(h)

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

	return systemHTTP
}
