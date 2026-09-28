package normalize

import (
	"fmt"
	"strings"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

// MaxRetryAttempts bounds retry amplification against a struggling upstream.
const MaxRetryAttempts = 5

// Services normalizes service definitions into their canonical
// intermediate representation.
func Services(services schema.Services) (*ir.Services, error) {
	if services == nil {
		return nil, fmt.Errorf("services must be provided: got nil map")
	}

	normalized := make(ir.Services, len(services))

	for name, service := range services {
		attempts := service.Retries.Attempts
		switch {
		case attempts < 0 || attempts > MaxRetryAttempts:
			return nil, fmt.Errorf(
				"service %q: retries.attempts must be between 0 and %d, got %d",
				name, MaxRetryAttempts, attempts,
			)
		case attempts == 0:
			attempts = 1
		}

		normalized[name] = ir.Service{
			Upstream: ir.Upstream{
				Scheme: strings.ToLower(service.Upstream.Scheme),
				Host:   strings.TrimSpace(service.Upstream.Host),
				Port:   service.Upstream.Port,
			},
			Retries: ir.Retries{Attempts: attempts},
		}
	}

	return &normalized, nil
}
