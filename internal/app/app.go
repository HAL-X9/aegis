package app

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/aegis/internal/config"
	"github.com/aegis/internal/controlplane/loader"
)

var runtimeConfigPath string
var routesConfigPath string

func init() {
	flag.StringVar(&runtimeConfigPath, "config", "", "path to app config (overrides env)")
	flag.StringVar(&routesConfigPath, "routes", "", "path to routes config (overrides env)")
}

// App is the process composition root: configuration load, bootstrap, and HTTP lifecycle.
type App struct {
	runtime *Runtime
}

// New parses flags, loads app configuration, bootstraps dependencies, and returns an App.
func New() (*App, error) {
	flag.Parse()

	runtimeConfigFile, err := config.ResolvePath(runtimeConfigPath, config.EnvRuntimeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("config.resolve runtime path: %w", err)
	}

	runtimeConfig, err := config.Load(runtimeConfigFile)
	if err != nil {
		return nil, fmt.Errorf("config.load runtime config (%s): %w", runtimeConfigFile, err)
	}

	routesConfigFile, err := config.ResolvePath(routesConfigPath, config.EnvRoutesConfigPath)
	if err != nil {
		return nil, fmt.Errorf("config.resolve routes path: %w", err)
	}

	gatewayConfig, err := loader.Load(routesConfigFile)
	if err != nil {
		return nil, fmt.Errorf("controlplane.loader.load manifest (%s): %w", routesConfigFile, err)
	}

	deps, err := Bootstrap(runtimeConfig, gatewayConfig)
	if err != nil {
		return nil, fmt.Errorf("app.bootstrap dependencies: %w", err)
	}

	public, err := NewHTTPServerComponent("public", deps.PublicHTTP)
	if err != nil {
		return nil, fmt.Errorf("init public server: %w", err)
	}
	system, err := NewHTTPServerComponent("system", deps.SystemHTTP)
	if err != nil {
		return nil, fmt.Errorf("init system server: %w", err)
	}

	lc := NewLifecycle(public, system, deps.Health)
	rt := NewRuntime(lc, 15*time.Second)

	return &App{runtime: rt}, nil
}

func (p *App) Run(ctx context.Context) error {
	if p == nil || p.runtime == nil {
		return fmt.Errorf("program is not initialized")
	}
	return p.runtime.Run(ctx)
}

func (p *App) Close() error {
	if p == nil || p.runtime == nil {
		return nil
	}
	return p.runtime.Close()
}
