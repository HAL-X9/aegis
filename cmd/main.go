package main

import (
	"context"
	"flag"
	"log"

	"github.com/aegis/internal/config"
	runtimeCfg "github.com/aegis/internal/runtime/config"
)

func main() {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	var cliPath string
	flag.StringVar(&cliPath, "config", "", "path to runtime config (overrides env)")
	flag.Parse()

	path, err := config.ResolvePath(cliPath, config.EnvRuntimeConfigPath)
	if err != nil {
		log.Fatalf("failed to resolve configuration path: %v", err)
	}

	_, err = runtimeCfg.LoadRuntimeConfig(path)
	if err != nil {
		log.Fatalf("failed to load runtime config: %v", err)
	}

}
