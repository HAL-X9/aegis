package aegis

import (
	"fmt"
	"net/http"

	runtimeconfig "github.com/aegis/internal/config/runtime"
	"github.com/aegis/internal/observe/health"
	httptransport "github.com/aegis/internal/transport/http"
)

// Dependencies is the constructed object graph for this process (explicit, no container).
type Dependencies struct {
	Runtime *runtimeconfig.Runtime
	Health  *health.Health
	HTTP    *http.Server
}

// Bootstrap wires configuration into concrete implementations. It does not start listeners.
func Bootstrap(cfg *runtimeconfig.Runtime) (*Dependencies, error) {
	if cfg == nil {
		return nil, fmt.Errorf("runtime config is nil")
	}

	hsvc := health.NewHealth()
	h := httptransport.NewHandler(hsvc)
	mux := httptransport.NewMux(h)

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           mux,
		ReadTimeout:       cfg.HTTP.Timeouts.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.Timeouts.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.Timeouts.WriteTimeout,
		IdleTimeout:       cfg.HTTP.Timeouts.IdleTimeout,
		TLSConfig:         cfg.HTTP.TLS,
		MaxHeaderBytes:    cfg.HTTP.MaxHeaderBytes,
	}

	return &Dependencies{
		Runtime: cfg,
		Health:  hsvc,
		HTTP:    srv,
	}, nil
}
