package config

import (
	"fmt"
)

// Load reads the YAML file at path, unmarshals it into Runtime, and runs Validate.
// On success the returned value is safe for use by the process app layer.
func Load(path string) (*Runtime, error) {
	cfg, err := ReadAndDecodeYaml[Runtime](path)
	if err != nil {
		return nil, fmt.Errorf("failed to load app configuration from YAML: %w", err)
	}

	if err = Validate(cfg); err != nil {
		return nil, fmt.Errorf("failed to validate app configuration: %w", err)
	}

	return cfg, nil
}
