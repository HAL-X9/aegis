package normalize

import (
	"fmt"
	"strings"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

// Services normalizes service definitions into their canonical
// intermediate representation.
func Services(services schema.Services) (*ir.Services, error) {
	if services == nil {
		return nil, fmt.Errorf("services must be provided: got nil map")
	}

	normalized := make(ir.Services, len(services))

	for name, service := range services {
		normalized[name] = ir.Service{
			Upstream: ir.Upstream{
				Scheme: strings.ToLower(service.Upstream.Scheme),
				Host:   strings.TrimSpace(service.Upstream.Host),
				Port:   service.Upstream.Port,
			},
		}
	}

	return &normalized, nil
}
