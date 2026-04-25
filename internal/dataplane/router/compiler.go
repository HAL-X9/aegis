package router

import (
	"fmt"

	"github.com/aegis/internal/config/controlplane"
)

// Compile transforms control-plane configuration into a routing manifest that
// is optimized for deterministic dataplane lookup.
func Compile(cfg *controlplane.AegisManifest) (*CompiledManifest, error) {
	if cfg == nil {
		return nil, fmt.Errorf("compile routing configuration: manifest is nil")
	}

	_ = make([]CompiledRoute, 0, len(cfg.Routes))
	// TODO: BitMash for Method and Header Predicates

	/*
		routes = append(routes, CompiledRoute{

		})
	*/

	return nil, nil
}
