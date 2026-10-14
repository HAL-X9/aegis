package pipeline

import (
	"fmt"

	"github.com/HAL-X9/aegis/internal/controlplane/compile"
	"github.com/HAL-X9/aegis/internal/controlplane/normalize"
	"github.com/HAL-X9/aegis/internal/controlplane/validate"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

// Build compiles the gateway configuration into a runtime snapshot.
//
// The build process includes route and policy compilation.
// The input configuration is not mutated.
func Build(validated validate.Validated) (*snapshot.CompiledConfig, error) {
	config := validated.Config()
	if config == nil {
		return nil, fmt.Errorf("build compiled configuration: gateway config is nil")
	}

	normalizedServices := normalize.Services(config.Services)
	normalizedRoutes := normalize.Routes(config.Routes)
	normalizedPolicies := normalize.Policies(&config.Policies)

	services, serviceIDs := compile.Services(normalizedServices)

	headerIDs := compile.NewHeaderRegistryBuilder()

	routes, err := compile.Routes(
		headerIDs,
		serviceIDs,
		normalizedRoutes,
		normalizedPolicies,
	)
	if err != nil {
		return nil, fmt.Errorf("build compiled configuration: route build failed: %w", err)
	}

	compile.ApplyServicePolicies(routes, services)

	index, err := compile.Index(routes)
	if err != nil {
		return nil, fmt.Errorf("build compiled configuration: route index failed: %w", err)
	}

	policies, err := compile.Policies(normalizedPolicies)
	if err != nil {
		return nil, fmt.Errorf("build compiled configuration: policy build failed: %w", err)
	}

	return &snapshot.CompiledConfig{
		Services:    services,
		Routes:      routes,
		Policies:    *policies,
		HeaderNames: headerIDs.Registry(),
		Index:       index,
	}, nil
}
