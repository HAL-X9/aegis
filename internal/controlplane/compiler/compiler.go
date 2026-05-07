package compiler

import (
	"fmt"
	"net"
	"net/url"
	"sort"
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

		// Compile and normalize header match constraints once during policy build
		// so request-path evaluation remains deterministic and allocation-light.
		headersPredicate, err := BuildHeadersPredicate(route.Match.Headers)
		if err != nil {
			return nil, fmt.Errorf("route %q: compile headers predicate: %w", route.Name, err)
		}

		// Upstream is converted into a fully qualified origin URL.
		// This removes the need for runtime URL construction in dataplane.
		upstream := BuildUpstreamOriginURL(route)

		compiledRoute = append(compiledRoute, CompiledRoute{
			Name: route.Name, // preserve route identity for debugging/observability

			Match: CompiledMatch{
				PathPrefix: pathPrefix,
				Methods:    methodMask,
				Headers:    headersPredicate,
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

// BuildHeadersPredicate validates and compiles route header constraints into a
// deterministic predicate slice for dataplane matching.
//
// Compilation guarantees:
//   - stable predicate order (sorted by header name)
//   - non-empty header names
//   - no empty allowed-value entries
//   - duplicate allowed values removed while preserving first-seen order
//
// Predicate semantics:
//   - nil or empty input map => no header constraints (nil, nil)
//   - empty value list for a header => presence-only constraint
//   - non-empty value list => exact-value whitelist (any request value may match)
//
// The returned slice is runtime-ready and must be treated as immutable.
func BuildHeadersPredicate(headers map[string][]string) ([]HeaderPredicate, error) {
	if len(headers) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(headers))
	for k := range headers {
		if k == "" {
			return nil, fmt.Errorf("header name cannot be empty")
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	predicates := make([]HeaderPredicate, 0, len(keys))

	for _, name := range keys {
		values := headers[name]
		if len(values) == 0 {
			predicates = append(predicates, HeaderPredicate{
				Name:          name,
				AllowedValues: nil,
			})
			continue
		}

		uniq := make(map[string]struct{}, len(values))
		clean := make([]string, 0, len(values))
		for _, v := range values {
			if v == "" {
				return nil, fmt.Errorf("header %q contains empty allowed value", name)
			}
			if _, ok := uniq[v]; ok {
				continue
			}
			uniq[v] = struct{}{}
			clean = append(clean, v)
		}

		predicates = append(predicates, HeaderPredicate{
			Name:          name,
			AllowedValues: clean,
		})
	}
	return predicates, nil
}
