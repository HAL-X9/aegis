package validate

import (
	"fmt"
	"strings"

	"github.com/aegis/internal/controlplane/model"
)

// allowedHTTPMethods defines the set of HTTP methods accepted by route validation.
var allowedHTTPMethods = map[string]struct{}{
	"GET":     {},
	"POST":    {},
	"PUT":     {},
	"DELETE":  {},
	"PATCH":   {},
	"OPTIONS": {},
	"HEAD":    {},
}

// Validate validates the gateway configuration and returns an error if it is invalid.
func Validate(cfg *model.GatewayConfig) error {
	if cfg == nil {
		return fmt.Errorf("gateway config validation failed: config is nil")
	}

	if err := validateRoutes(cfg.Routes); err != nil {
		return fmt.Errorf("gateway config validation failed: routes are invalid: %w", err)
	}

	return nil
}

// validateRoutes validates all configured routes and reports the index of the first invalid route.
func validateRoutes(routes []model.Route) error {
	if routes == nil {
		return fmt.Errorf("routes must be provided: got nil slice")
	}

	for i, route := range routes {
		if err := validateRoute(&route); err != nil {
			return fmt.Errorf("invalid route at index %d: %w", i, err)
		}
	}
	return nil
}

// validateRoute validates a single route, including identity, match rules, and upstream endpoint.
func validateRoute(route *model.Route) error {
	if route == nil {
		return fmt.Errorf("route validation failed: route is nil")
	}

	if strings.TrimSpace(route.Name) == "" {
		return fmt.Errorf("route validation failed: name is required and must not be blank")
	}

	if strings.TrimSpace(route.Match.PathPrefix) == "" {
		return fmt.Errorf("route validation failed: match.path_prefix is required and must not be blank")
	}

	if route.Match.PathPrefix[0] != '/' {
		return fmt.Errorf("route validation failed: match.path_prefix must start with '/'")
	}

	for _, method := range route.Match.Methods {
		if _, ok := allowedHTTPMethods[method]; !ok {
			return fmt.Errorf("route validation failed: match.methods contains unsupported method %q", method)
		}
	}

	if strings.TrimSpace(route.Upstream.Host) == "" {
		return fmt.Errorf("route validation failed: upstream.host is required and must not be blank")
	}

	if route.Upstream.Port <= 0 || route.Upstream.Port > 65535 {
		return fmt.Errorf("route validation failed: upstream.port must be in range 1..65535")
	}

	return nil
}
