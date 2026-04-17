package server

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/aegis/internal/runtime/config"
)

type Application struct {
	config       *config.Runtime
	http         *http.Server
	shutdownOnce sync.Once
}

func NewApplication(cfg *config.Runtime) (*Application, error) {
	if cfg == nil {
		return nil, fmt.Errorf("runtime config is nil")
	}
	return &Application{
		config: cfg,
		http:   newHTTPServer(cfg),
	}, nil
}

func newHTTPServer(cfg *config.Runtime) *http.Server {
	return &http.Server{
		Addr:              cfg.HTTP.Addr,
		ReadTimeout:       cfg.HTTP.Timeouts.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.Timeouts.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.Timeouts.WriteTimeout,
		IdleTimeout:       cfg.HTTP.Timeouts.IdleTimeout,
		TLSConfig:         cfg.HTTP.TLS,
		MaxHeaderBytes:    cfg.HTTP.MaxHeaderBytes,
	}
}
