package public

import (
	"net/http"

	"github.com/HAL-X9/aegis/internal/config"
)

func NewPublicServer(cfg *config.Runtime, executor RequestExecutor) *http.Server {
	forward := NewForwardHandler(executor)
	publicHandler := NewRouter(forward)

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

	return publicHTTP
}
