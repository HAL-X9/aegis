package runtimeconfig

import (
	"fmt"

	"github.com/aegis/internal/config/loader"
)

func Load(path string) (*Runtime, error) {
	cfg, err := loader.ReadAndDecodeYaml[Runtime](path)
	if err != nil {
		return nil, fmt.Errorf("failed to load runtime configuration from YAML: %w", err)
	}

	if err = Validate(cfg); err != nil {
		return nil, fmt.Errorf("failed to validate runtime configuration: %w", err)
	}

	return cfg, nil
}
