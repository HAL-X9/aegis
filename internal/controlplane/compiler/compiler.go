package compiler

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/aegis/internal/contracts/methodmask"
	"github.com/aegis/internal/controlplane/model"
)

// Compile transforms control-plane configuration into an immutable
// routing manifest optimized for dataplane execution.
//
// This is a control-plane operation and must NOT be used in the dataplane.
//
// Key properties of the output:
//   - fully precomputed (no parsing in runtime path)
//   - deterministic routing rules
//   - optimized for fast lookup and evaluation
func Compile(cfg *model.GatewayConfig) (*CompiledGatewayConfig, error) {
	if cfg == nil {
		return nil, fmt.Errorf("compile routing configuration: manifest is nil")
	}

	// Preallocate slice to avoid dynamic growth during compilation.
	compiledRoute := make([]CompiledRoute, 0, len(cfg.Routes))

	for _, route := range cfg.Routes {

		// Path prefix is taken as-is, assuming it has already been normalized
		// in the validation/normalization stage of the control plane.
		pathPrefix := route.Match.PathPrefix

		// Convert human-readable HTTP methods into a bitmask representation
		// for efficient O(1) matching in dataplane hot path.
		methodMask, err := methodmask.BuildMethodMask(route.Match.Methods)
		if err != nil {
			return nil, fmt.Errorf("route %q: compile method mask: %w", route.Name, err)
		}

		// Upstream is converted into a fully qualified origin URL.
		// This removes the need for runtime URL construction in dataplane.
		upstream := BuildUpstreamOriginURL(route)

		compiledRoute = append(compiledRoute, CompiledRoute{
			Name: route.Name, // preserve route identity for debugging/observability

			Match: CompiledMatch{
				PathPrefix: pathPrefix,
				Methods:    methodMask,
			},

			Upstream: upstream,
		})
	}

	return &CompiledGatewayConfig{Routes: compiledRoute}, nil
}

// BuildUpstreamOriginURL converts a control-plane upstream definition into a
// fully qualified origin URL string.
//
// This operation is intentionally done at compile time to avoid:
//
//   - repeated string formatting in dataplane hot path
//   - runtime allocation overhead
//   - ambiguity in scheme/host/port resolution
func BuildUpstreamOriginURL(route model.Route) string {
	u := &url.URL{
		Scheme: route.Upstream.Scheme,

		// Host is constructed as "host:port" to match standard HTTP origin format.
		// net.JoinHostPort ensures correct IPv6 handling.
		Host: net.JoinHostPort(
			route.Upstream.Host,
			strconv.Itoa(route.Upstream.Port),
		),
	}

	return u.String()
}
