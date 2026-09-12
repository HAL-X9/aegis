package app

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/HAL-X9/aegis/internal/config"
	"github.com/HAL-X9/aegis/internal/controlplane/loader"
)

// App is the process composition root: configuration load, dependency
// bootstrap, and HTTP lifecycle, wired together in that order.
//
// An App is meant to be constructed once via New and run exactly once via
// Run. Run owns the full lifetime of everything New built: starting it,
// blocking until shutdown is requested or a dependency fails, and stopping
// it again. Nothing outside App needs to close anything once Run returns —
// there is deliberately no App.Close.
type App struct {
	lifecycle *Lifecycle
}

// New parses CLI flags, loads runtime and routes configuration, and
// bootstraps all dependencies (routing engine, executor, HTTP servers).
// It performs no network I/O: no listener is opened until Run is called.
func New() (*App, error) {
	runtimeConfigPath, routesConfigPath, err := parseFlags(os.Args[1:])
	if err != nil {
		return nil, fmt.Errorf("parse flags: %w", err)
	}

	runtimeConfigFile, err := config.ResolvePath(runtimeConfigPath, config.EnvRuntimeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime config path: %w", err)
	}

	runtimeConfig, err := config.Load(runtimeConfigFile)
	if err != nil {
		return nil, fmt.Errorf("load runtime config (%s): %w", runtimeConfigFile, err)
	}

	routesConfigFile, err := config.ResolvePath(routesConfigPath, config.EnvRoutesConfigPath)
	if err != nil {
		return nil, fmt.Errorf("resolve routes config path: %w", err)
	}

	gatewayManifest, err := loader.Load(routesConfigFile)
	if err != nil {
		return nil, fmt.Errorf("load routes manifest (%s): %w", routesConfigFile, err)
	}

	deps, err := Bootstrap(runtimeConfig, gatewayManifest)
	if err != nil {
		return nil, fmt.Errorf("bootstrap dependencies: %w", err)
	}

	lifecycle, err := NewLifecycle(deps)
	if err != nil {
		return nil, fmt.Errorf("init lifecycle: %w", err)
	}

	return &App{lifecycle: lifecycle}, nil
}

// Run starts every HTTP listener, blocks until ctx is canceled or a
// listener fails, then stops everything gracefully before returning.
// Callers must not call Run more than once on the same App.
func (a *App) Run(ctx context.Context) error {
	return a.lifecycle.Run(ctx)
}

// parseFlags parses args into a private FlagSet rather than registering
// onto the global flag.CommandLine.
//
// The package-level vars + init() this replaces mutated process-global
// state exactly once, at package init: calling app.New a second time in
// the same process (a table-driven test doing so, for instance) would have
// panicked with "flag redefined". A FlagSet owned by this call has no such
// failure mode, and New no longer depends on init() order at all.
func parseFlags(args []string) (runtimeConfigPath, routesConfigPath string, err error) {
	fs := flag.NewFlagSet("aegis", flag.ContinueOnError)
	fs.StringVar(&runtimeConfigPath, "config", "", "path to app config (overrides env)")
	fs.StringVar(&routesConfigPath, "routes", "", "path to routes config (overrides env)")
	if err = fs.Parse(args); err != nil {
		return "", "", err
	}
	return runtimeConfigPath, routesConfigPath, nil
}
