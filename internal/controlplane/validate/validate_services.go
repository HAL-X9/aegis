package validate

import (
	"fmt"
	"strings"

	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

func validateServices(services schema.Services) error {
	for name, service := range services {
		if err := validateService(service); err != nil {
			return fmt.Errorf("invalid service %q: %w", name, err)
		}
	}

	return nil
}

func validateService(service schema.Service) error {
	if strings.TrimSpace(service.Upstream.Scheme) == "" {
		return fmt.Errorf("upstream.scheme is required and must be non-empty")
	}

	switch service.Upstream.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("upstream.scheme must be one of: http, https")
	}

	if strings.TrimSpace(service.Upstream.Host) == "" {
		return fmt.Errorf("route validation failed: upstream.host is required and must not be blank")
	}

	if service.Upstream.Port <= 0 || service.Upstream.Port > 65535 {
		return fmt.Errorf("route validation failed: upstream.port must be in range 1..65535")
	}

	return nil
}
