package app

import (
	"context"
	"flag"
	"fmt"

	"github.com/aegis/internal/config/loader"
	runtimeconfig "github.com/aegis/internal/config/runtime"
)

var configPath string

func init() {
	flag.StringVar(&configPath, "config", "", "path to runtime config (overrides env)")
}

// App is the process composition root: configuration load, bootstrap, and HTTP lifecycle.
type App struct {
	http *HTTPServer
}

// New parses flags, loads runtime configuration, bootstraps dependencies, and returns an App.
func New() (*App, error) {
	flag.Parse()

	path, err := loader.ResolvePath(configPath, loader.EnvRuntimeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("resolve configuration path: %w", err)
	}

	cfg, err := runtimeconfig.Load(path)
	if err != nil {
		return nil, err
	}

	deps, err := Bootstrap(cfg)
	if err != nil {
		return nil, err
	}

	httpsrv, err := NewHTTPServer(deps.HTTP)
	if err != nil {
		return nil, err
	}

	return &App{http: httpsrv}, nil
}

func (a *App) Run(ctx context.Context) error {
	if a == nil || a.http == nil {
		return fmt.Errorf("app is not initialized")
	}
	return a.http.Run(ctx)
}

func (a *App) Close() error {
	if a == nil || a.http == nil {
		return nil
	}
	return a.http.Close()
}
