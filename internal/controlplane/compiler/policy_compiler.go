package compiler

import (
	"fmt"

	"github.com/aegis/internal/controlplane/model"
)

func CompilePolicy(cfg *model.Policies) (*CompiledPolicies, error) {
	if cfg == nil {
		return nil, fmt.Errorf("compile policies configuration: config is nil")
	}

	return nil, nil
}
