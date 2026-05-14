package compiler

import (
	"fmt"

	"github.com/aegis/internal/controlplane/normalize"
)

func CompilePolicy(policies *normalize.NormalizedPolicies) (*CompiledPolicies, error) {
	if policies == nil {
		return nil, fmt.Errorf("compile policies configuration: config is nil")
	}

	// TODO: Normalized to Compiled

	// Preallocate slice to avoid dynamic growth during compilation.
	// compiledHeaders := make([]CompiledHeaders, 0, len(policies.Headers))

	return nil, nil
}
