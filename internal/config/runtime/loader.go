package runtimeconfig

import (
	"fmt"

	"github.com/aegis/internal/config/loader"
)

// Load reads the YAML file at path, unmarshals it into Runtime, and runs Validate.
// On success the returned value is safe for use by the process runtime layer.
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
