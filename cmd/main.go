package main

import (
	"context"
	"errors"
	"flag"
	"log"

	"github.com/aegis/internal/config"
	runtimeCfg "github.com/aegis/internal/runtime/config"
	"github.com/aegis/internal/runtime/server"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var cliPath string
	flag.StringVar(&cliPath, "config", "", "path to runtime config (overrides env)")
	flag.Parse()

	path, err := config.ResolvePath(cliPath, config.EnvRuntimeConfigPath)
	if err != nil {
		log.Fatalf("failed to resolve configuration path: %v", err)
	}

	cfg, err := runtimeCfg.LoadRuntimeConfig(path)
	if err != nil {
		log.Fatalf("failed to load runtime config: %v", err)
	}

	app, err := server.NewApplication(cfg)
	if err != nil {
		log.Fatalf("failed to create container: %v", err)
	}
	defer func() {
		if err = app.Close(); err != nil {
			log.Printf("container close error: %v", err)
		}
	}()

	if err = app.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("failed to run application: %v", err)
	}
}
