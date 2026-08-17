package pipeline

import (
	"fmt"

	"github.com/HAL-X9/aegis/internal/controlplane/compile"
	"github.com/HAL-X9/aegis/internal/controlplane/normalize"
	"github.com/HAL-X9/aegis/internal/controlplane/schema"
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

// Build compiles the gateway configuration into a runtime snapshot.
//
// The build process includes route and policy compilation.
// The input configuration is not mutated.
func Build(config *schema.GatewayConfig) (*snapshot.CompiledConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("build compiled configuration: gateway config is nil")
	}

	normalizedServices, err := normalize.Services(config.Services)
	if err != nil {
		return nil, fmt.Errorf("build compiled configuration: service normalization failed: %w", err)
	}

	normalizedRoutes := normalize.Routes(config.Routes)

	normalizedPolicies, err := normalize.Policies(&config.Policies)
	if err != nil {
		return nil, fmt.Errorf("build compiled configuration: policy normalization failed: %w", err)
	}

	services, serviceIDs, err := compile.Services(*normalizedServices)
	if err != nil {
		return nil, fmt.Errorf("build compiled configuration: service compilation failed: %w", err)
	}

	routes, err := compile.Routes(serviceIDs, normalizedRoutes, normalizedPolicies)
	if err != nil {
		return nil, fmt.Errorf("build compiled configuration: route build failed: %w", err)
	}

	policies, err := compile.Policies(normalizedPolicies)
	if err != nil {
		return nil, fmt.Errorf("build compiled configuration: policy build failed: %w", err)
	}

	return &snapshot.CompiledConfig{
		Services: services,
		Routes:   routes,
		Policies: *policies,
	}, nil
}
