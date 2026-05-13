package validate

import (
	"fmt"
	"strings"

	"github.com/aegis/internal/controlplane/schema"
)

// allowedHTTPMethods defines the set of supported HTTP methods
// accepted in route match configuration.
var allowedHTTPMethods = map[string]struct{}{
	"GET":     {},
	"POST":    {},
	"PUT":     {},
	"DELETE":  {},
	"PATCH":   {},
	"OPTIONS": {},
	"HEAD":    {},
}

// validateRoutes validates a collection of routes.
// It returns the first encountered validation error with the corresponding index.
func validateRoutes(routes []schema.Route) error {
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

// validateRoute performs validation of a single route definition,
// including identity, match conditions, and upstream configuration.
func validateRoute(route *schema.Route) error {
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

	for k := range route.Match.Headers {
		if strings.TrimSpace(k) == "" {
			return fmt.Errorf("invalid header key: empty string")
		}

		if len(k) > 256 {
			return fmt.Errorf("invalid header key %q: exceeds 256 characters", k)
		}
	}

	if strings.TrimSpace(route.Upstream.Scheme) == "" {
		return fmt.Errorf("upstream.scheme is required and must be non-empty")
	}

	switch route.Upstream.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("upstream.scheme must be one of: http, https")
	}

	if strings.TrimSpace(route.Upstream.Host) == "" {
		return fmt.Errorf("route validation failed: upstream.host is required and must not be blank")
	}

	if route.Upstream.Port <= 0 || route.Upstream.Port > 65535 {
		return fmt.Errorf("route validation failed: upstream.port must be in range 1..65535")
	}

	return nil
}
