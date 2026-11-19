package validate

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

// maxRetryAttempts bounds retry amplification against a struggling upstream.
const maxRetryAttempts = 5

func validateServices(services schema.Services) error {
	for name, service := range services {
		if err := validateService(service); err != nil {
			return fmt.Errorf("invalid service %q: %w", name, err)
		}
	}

	return nil
}

func validateService(service schema.Service) error {
	if err := validateUpstream(service.Upstream); err != nil {
		return err
	}

	if service.Timeout < 0 {
		return fmt.Errorf("timeout must not be negative")
	}

	if service.Retries.Attempts < 0 || service.Retries.Attempts > maxRetryAttempts {
		return fmt.Errorf("retries.attempts must be between 0 and %d, got %d", maxRetryAttempts, service.Retries.Attempts)
	}

	if service.Retries.AttemptTimeout < 0 {
		return fmt.Errorf("retries.attempt_timeout must not be negative")
	}

	if service.Timeout > 0 && service.Retries.AttemptTimeout > service.Timeout {
		return fmt.Errorf("retries.attempt_timeout must not exceed timeout")
	}

	return nil
}

// validateUpstream requires a supported scheme and at least one target, each
// a host and a port, with no endpoint listed twice. A repeated endpoint would
// silently receive a larger share of the traffic.
func validateUpstream(upstream schema.Upstream) error {
	switch upstream.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("upstream.scheme must be one of: http, https")
	}

	if len(upstream.Targets) == 0 {
		return fmt.Errorf("upstream.targets must list at least one target")
	}

	seen := make(map[string]int, len(upstream.Targets))
	for i, target := range upstream.Targets {
		host := strings.TrimSpace(target.Host)
		if host == "" {
			return fmt.Errorf("upstream.targets[%d].host must not be blank", i)
		}

		if target.Port <= 0 || target.Port > 65535 {
			return fmt.Errorf("upstream.targets[%d].port must be in range 1..65535", i)
		}

		endpoint := net.JoinHostPort(host, strconv.Itoa(target.Port))
		if first, dup := seen[endpoint]; dup {
			return fmt.Errorf("upstream.targets[%d] repeats upstream.targets[%d] (%s)", i, first, endpoint)
		}
		seen[endpoint] = i
	}

	return nil
}
