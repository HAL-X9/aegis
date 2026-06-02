package pipeline

import (
	"fmt"

	"github.com/aegis/internal/controlplane/compile"
	"github.com/aegis/internal/controlplane/normalize"
	"github.com/aegis/internal/controlplane/schema"
	"github.com/aegis/internal/controlplane/snapshot"
)

// Build compiles the gateway configuration into a runtime snapshot.
//
// The build process includes route and policy compilation.
// The input configuration is not mutated.
func Build(config *schema.GatewayConfig) (*snapshot.CompiledConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("build compiled configuration: gateway config is nil")
	}

	normalizedRoutes := normalize.Routes(config.Routes)

	normalizedPolicies, err := normalize.Policies(&config.Policies)
	if err != nil {
		return nil, fmt.Errorf("build compiled configuration: policy normalization failed: %w", err)
	}

	routes, err := compile.Routes(normalizedRoutes, normalizedPolicies)
	if err != nil {
		return nil, fmt.Errorf("build compiled configuration: route build failed: %w", err)
	}

	policies, err := compile.Policy(normalizedPolicies)
	if err != nil {
		return nil, fmt.Errorf("build compiled configuration: policy build failed: %w", err)
	}

	return &snapshot.CompiledConfig{
		Routes:   routes,
		Policies: *policies,
	}, nil
}

// BuildRoutes normalizes and compiles route definitions into executable routes.
//
// The input configuration is not mutated.
func BuildRoutes(config *schema.GatewayConfig) ([]snapshot.CompiledRoute, error) {
	if config == nil {
		return nil, fmt.Errorf("compile routes configuration: gateway config is nil")
	}

	normalizedRoutes := normalize.Routes(config.Routes)

	compiledRoutes, err := compile.Routes(normalizedRoutes, nil)
	if err != nil {
		return nil, fmt.Errorf("compile routes configuration: route compilation failed: %w", err)
	}

	return compiledRoutes, nil
}

// BuildPolicies compiles a policy source into an internal executable representation.
//
// The process is deterministic and consists of two stages:
//  1. Normalization: validates and converts the input source into a canonical intermediate form.
//  2. Compilation: transforms the normalized representation into a runtime-ready compiled policy model.
//
// The function is side effect free and does not mutate the input source. It returns an error if either representation fails.
func BuildPolicies(schema *schema.Policies) (*snapshot.CompiledPolicies, error) {
	normalizedPolicies, err := normalize.Policies(schema)
	if err != nil {
		return nil, fmt.Errorf("compile policies configuration: normalization failed: %w", err)
	}

	compiledPolicies, err := compile.Policy(normalizedPolicies)
	if err != nil {
		return nil, fmt.Errorf("compile policies configuration: policy compilation failed after normalization: %w", err)
	}

	return compiledPolicies, nil
}
