package normalize

import (
	"strings"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

// Services normalizes service definitions into their canonical
// intermediate representation. The input must have passed validation.
func Services(services schema.Services) ir.Services {
	normalized := make(ir.Services, len(services))

	for name, service := range services {
		attempts := service.Retries.Attempts
		if attempts == 0 {
			attempts = 1
		}

		normalized[name] = ir.Service{
			Upstream: upstream(service.Upstream),
			Timeout:  service.Timeout,
			Retries: ir.Retries{
				Attempts:       attempts,
				AttemptTimeout: service.Retries.AttemptTimeout,
			},
		}
	}

	return normalized
}

// upstream keeps the targets in configuration order, so the order in which
// the data plane cycles through them is the order the operator wrote.
func upstream(upstream schema.Upstream) ir.Upstream {
	targets := make([]ir.Target, len(upstream.Targets))
	for i, target := range upstream.Targets {
		targets[i] = ir.Target{
			Host: strings.TrimSpace(target.Host),
			Port: target.Port,
		}
	}

	return ir.Upstream{
		Scheme:  strings.ToLower(upstream.Scheme),
		Targets: targets,
	}
}
