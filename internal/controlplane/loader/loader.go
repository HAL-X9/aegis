package loader

import (
	"fmt"

	"github.com/aegis/internal/config"
	"github.com/aegis/internal/controlplane/model"
	"github.com/aegis/internal/controlplane/validate"
)

// Load reads the YAML file at path, unmarshals it into AegisManifest, and runs Validate.
// On success the returned value is safe for use by the gateway control-plane layer.
func Load(path string) (*model.GatewayConfig, error) {
	cfg, err := config.ReadAndDecodeYaml[model.GatewayConfig](path)
	if err != nil {
		return nil, fmt.Errorf("failed to load controlplane configuration from YAML: %w", err)
	}

	if err = validate.Validate(cfg); err != nil {
		return nil, fmt.Errorf("failed to validate controlplane configuration: %w", err)
	}

	return cfg, nil
}
