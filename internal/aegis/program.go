package aegis

import (
	"context"
	"flag"
	"fmt"

	"github.com/aegis/internal/config"
	"github.com/aegis/internal/controlplane/loader"
)

var runtimeConfigPath string
var routesConfigPath string

func init() {
	flag.StringVar(&runtimeConfigPath, "config", "", "path to app config (overrides env)")
	flag.StringVar(&routesConfigPath, "routes", "", "path to routes config (overrides env)")
}

// Program is the process composition root: configuration load, bootstrap, and HTTP lifecycle.
type Program struct {
	http *serverGroup
}

// New parses flags, loads app configuration, bootstraps dependencies, and returns a Program.
func New() (*Program, error) {
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
		return nil, fmt.Errorf("aegis.bootstrap dependencies: %w", err)
	}

	httpServers, err := newServerGroup(deps.PublicHTTP, deps.SystemHTTP, deps.Health)
	if err != nil {
		return nil, fmt.Errorf("http.server.group.init: %w", err)
	}

	return &Program{http: httpServers}, nil
}

func (p *Program) Run(ctx context.Context) error {
	if p == nil || p.http == nil {
		return fmt.Errorf("program is not initialized")
	}
	return p.http.Run(ctx)
}

func (p *Program) Close() error {
	if p == nil || p.http == nil {
		return nil
	}
	return p.http.Close()
}
