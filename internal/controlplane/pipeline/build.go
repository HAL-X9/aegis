package pipeline

import (
	"fmt"

	"github.com/aegis/internal/controlplane/compile"
	"github.com/aegis/internal/controlplane/normalize"
	"github.com/aegis/internal/controlplane/schema"
	"github.com/aegis/internal/controlplane/snapshot"
)

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
