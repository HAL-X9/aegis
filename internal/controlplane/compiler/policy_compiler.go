package compiler

import (
	"fmt"

	"github.com/aegis/internal/controlplane/normalize"
	"github.com/aegis/internal/controlplane/schema"
)

func CompilePolicy(cfg *schema.Policies) (*CompiledPolicies, error) {
	if cfg == nil {
		return nil, fmt.Errorf("compile policies configuration: config is nil")
	}

	_, err := normalize.Normalize(cfg)
	if err != nil {
		return nil, fmt.Errorf("")
	}

	return nil, nil
}
