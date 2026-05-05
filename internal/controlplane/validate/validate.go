package validate

import (
	"fmt"

	"github.com/aegis/internal/controlplane/model"
)

func Validate(gatewayCfg *model.GatewayConfig) error {
	if gatewayCfg == nil {
		return fmt.Errorf("gateway configuration is nil")
	}

	if err := validateRoutes(gatewayCfg.Routes); err != nil {
		return fmt.Errorf("gateway config validation failed: routes are invalid: %w", err)
	}

	if err := validatesPolicies(&gatewayCfg.Policies); err != nil {
		return fmt.Errorf("gateway configuration validation failed: invalid policies: %w", err)
	}

	return nil
}
