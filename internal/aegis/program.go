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
	http *httpServer
}

// New parses flags, loads app configuration, bootstraps dependencies, and returns a Program.
func New() (*Program, error) {
	flag.Parse()

	runtimeConfigFile, err := config.ResolvePath(runtimeConfigPath, config.EnvRuntimeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("resolve configuration path: %w", err)
	}

	runtimeConfig, err := config.Load(runtimeConfigFile)
	if err != nil {
		return nil, err
	}

	routesConfigFile, err := config.ResolvePath(routesConfigPath, config.EnvRoutesConfigPath)
	if err != nil {
		return nil, err
	}

	aegisManifest, err := loader.Load(routesConfigFile)
	if err != nil {
		return nil, err
	}

	deps, err := Bootstrap(runtimeConfig, aegisManifest)
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
