package validate

import (
	"fmt"

	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

// Validate performs semantic validation for a gateway runtime configuration.
//
// It validates top-level route definitions and reusable policy sections and
// returns a descriptive error on the first detected violation.
func Validate(gatewayCfg *schema.GatewayConfig) error {
	if gatewayCfg == nil {
		return fmt.Errorf("gateway configuration is nil")
	}

	if err := validateServices(gatewayCfg.Services); err != nil {
		return fmt.Errorf("gateway config validation failed: services are invalid: %w", err)
	}

	if err := validateRoutes(gatewayCfg.Routes); err != nil {
		return fmt.Errorf("gateway config validation failed: routes are invalid: %w", err)
	}

	if err := validatePolicies(&gatewayCfg.Policies); err != nil {
		return fmt.Errorf("gateway configuration validation failed: invalid policies: %w", err)
	}

	return nil
}
