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
			Upstream: ir.Upstream{
				Scheme: strings.ToLower(service.Upstream.Scheme),
				Host:   strings.TrimSpace(service.Upstream.Host),
				Port:   service.Upstream.Port,
			},
			Timeout: service.Timeout,
			Retries: ir.Retries{
				Attempts:       attempts,
				AttemptTimeout: service.Retries.AttemptTimeout,
			},
		}
	}

	return normalized
}
