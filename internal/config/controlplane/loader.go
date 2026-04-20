package controlplane

import (
	"fmt"

	"github.com/aegis/internal/config/loader"
)

// Load reads the YAML file at path, unmarshals it into AegisManifest, and runs Validate.
// On success the returned value is safe for use by the gateway control-plane layer.
func Load(path string) (*AegisManifest, error) {
	cfg, err := loader.ReadAndDecodeYaml[AegisManifest](path)
	if err != nil {
		return nil, fmt.Errorf("failed to load controlplane configuration from YAML: %w", err)
	}

	if err = Validate(cfg); err != nil {
		return nil, fmt.Errorf("failed to validate controlplane configuration: %w", err)
	}

	return cfg, nil
}
