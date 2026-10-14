package validate

import (
	"fmt"

	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

// Validated is a gateway configuration that has passed Validate. Only
// Validate produces a non-zero Validated, and Validate checks every rule that
// depends on the configuration alone, including references between sections.
// Any consumer that accepts a Validated can rely on it being complete.
type Validated struct {
	config *schema.GatewayConfig
}

// Config returns the validated configuration. It must be treated as read-only.
func (v Validated) Config() *schema.GatewayConfig {
	return v.config
}

// Validate performs semantic validation for a gateway runtime configuration.
//
// It validates top-level route definitions and reusable policy sections and
// returns a descriptive error on the first detected violation.
func Validate(gatewayCfg *schema.GatewayConfig) (Validated, error) {
	if gatewayCfg == nil {
		return Validated{}, fmt.Errorf("gateway configuration is nil")
	}

	if gatewayCfg.Services == nil {
		return Validated{}, fmt.Errorf("gateway config validation failed: services must be provided: got nil map")
	}

	if err := validateServices(gatewayCfg.Services); err != nil {
		return Validated{}, fmt.Errorf("gateway config validation failed: services are invalid: %w", err)
	}

	if err := validatePolicies(&gatewayCfg.Policies); err != nil {
		return Validated{}, fmt.Errorf("gateway configuration validation failed: invalid policies: %w", err)
	}

	if err := validateRoutes(gatewayCfg.Routes); err != nil {
		return Validated{}, fmt.Errorf("gateway config validation failed: routes are invalid: %w", err)
	}

	if err := validateReferences(gatewayCfg.Routes, gatewayCfg.Services, &gatewayCfg.Policies); err != nil {
		return Validated{}, fmt.Errorf("gateway config validation failed: references are invalid: %w", err)
	}

	return Validated{config: gatewayCfg}, nil
}
