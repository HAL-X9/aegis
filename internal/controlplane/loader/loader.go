package loader

import (
	"fmt"

	"github.com/HAL-X9/aegis/internal/config"
	"github.com/HAL-X9/aegis/internal/controlplane/schema"
	"github.com/HAL-X9/aegis/internal/controlplane/validate"
)

// Load reads the YAML file at path, unmarshals it into GatewayConfig, and runs Validate.
// On success the returned value is safe for use by the gateway control-plane layer.
func Load(path string) (validate.Validated, error) {
	cfg, err := config.ReadAndDecodeYaml[schema.GatewayConfig](path)
	if err != nil {
		return validate.Validated{}, fmt.Errorf("failed to load controlplane configuration from YAML: %w", err)
	}

	validated, err := validate.Validate(cfg)
	if err != nil {
		return validate.Validated{}, fmt.Errorf("failed to validate controlplane configuration: %w", err)
	}

	return validated, nil
}
