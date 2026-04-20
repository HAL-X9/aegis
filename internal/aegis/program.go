package aegis

import (
	"context"
	"flag"
	"fmt"

	"github.com/aegis/internal/config/controlplane"
	"github.com/aegis/internal/config/loader"
	runtimeconfig "github.com/aegis/internal/config/runtime"
)

var runtimeConfigPath string
var routesConfigPath string

func init() {
	flag.StringVar(&runtimeConfigPath, "config", "", "path to runtime config (overrides env)")
	flag.StringVar(&routesConfigPath, "routes", "", "path to routes config (overrides env)")
}

// Program is the process composition root: configuration load, bootstrap, and HTTP lifecycle.
type Program struct {
	http *httpServer
}

// New parses flags, loads runtime configuration, bootstraps dependencies, and returns a Program.
func New() (*Program, error) {
	flag.Parse()

	runtimeConfigFile, err := loader.ResolvePath(runtimeConfigPath, loader.EnvRuntimeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("resolve configuration path: %w", err)
	}

	runtimeConfig, err := runtimeconfig.Load(runtimeConfigFile)
	if err != nil {
		return nil, err
	}

	routesConfigFile, err := loader.ResolvePath(routesConfigPath, loader.EnvRoutesConfigPath)
	if err != nil {
		return nil, err
	}

	_, err = controlplane.Load(routesConfigFile)
	if err != nil {
		return nil, err
	}

	deps, err := Bootstrap(runtimeConfig)
	if err != nil {
		return nil, err
	}

	httpsrv, err := newHTTPServer(deps.HTTP)
	if err != nil {
		return nil, err
	}

	return &Program{http: httpsrv}, nil
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
