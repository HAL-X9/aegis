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

	if service.Timeout < 0 {
		return fmt.Errorf("timeout must not be negative")
	}

	if service.Retries.AttemptTimeout < 0 {
		return fmt.Errorf("retries.attempt_timeout must not be negative")
	}

	if service.Timeout > 0 && service.Retries.AttemptTimeout > service.Timeout {
		return fmt.Errorf("retries.attempt_timeout must not exceed timeout")
	}

	return nil
}
