package compiler

import (
	"fmt"

	"github.com/aegis/internal/controlplane/model"
)

// Compile transforms control-plane configuration into a routing manifest that
// is optimized for deterministic dataplane lookup.
func Compile(cfg *model.GatewayConfig) (*CompiledGatewayConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("compile routing configuration: manifest is nil")
	}

	compiledRoute := make([]CompiledRoute, 0, len(cfg.Routes))
	for _, route := range cfg.Routes {
		compiledRoute = append(compiledRoute, CompiledRoute{
			PathPrefix: route.Match.PathPrefix,
			Upstream:   route.Upstream.Host,
		})
	}

	// TODO: BitMash for Method and Header Predicates

	/*
		routes = append(routes, CompiledRoute{

		})
	*/

	return &CompiledGatewayConfig{Routes: compiledRoute}, nil
}
