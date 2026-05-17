package compile

import (
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"

	"github.com/aegis/internal/contracts/methodmask"
	"github.com/aegis/internal/controlplane/ir"
	"github.com/aegis/internal/controlplane/snapshot"
)

// Routes transforms normalized route definitions into immutable route
// entries optimized for dataplane execution.
//
// This is a control-plane operation and must NOT be used in the dataplane.
//
// Key properties of the output:
//   - fully precomputed (no parsing in runtime path)
//   - deterministic routing rules
//   - optimized for fast lookup and evaluation
func Routes(routes []ir.Route) ([]snapshot.CompiledRoute, error) {
	compiledRoutes := make([]snapshot.CompiledRoute, 0, len(routes))

	for _, route := range routes {
		// Path prefix is taken as-is, assuming it has already been normalized
		// in the validation/normalization representation of the control plane.
		pathPrefix := route.Match.PathPrefix

		// Convert human-readable HTTP methods into a bitmask representation
		// for efficient O(1) matching in dataplane hot path.
		methodMask, err := methodmask.BuildMethodMask(route.Match.Methods)
		if err != nil {
			return nil, fmt.Errorf("route %q: compile method mask: %w", route.Name, err)
		}

		// Compile header match constraints once during control-plane compilation
		// so request-path evaluation remains deterministic and allocation-light.
		headerPredicates, err := headersPredicate(route.Match.Headers)
		if err != nil {
			return nil, fmt.Errorf("route %q: compile headers predicate: %w", route.Name, err)
		}

		// Upstream is converted into a fully qualified origin URL.
		// This removes the need for runtime URL construction in dataplane.
		upstream := upstreamOriginURL(route.Upstream)

		compiledRoutes = append(compiledRoutes, snapshot.CompiledRoute{
			Name: route.Name,

			Match: snapshot.CompiledMatch{
				PathPrefix: pathPrefix,
				Methods:    methodMask,
				Headers:    headerPredicates,
			},

			Upstream: upstream,
		})
	}

	return compiledRoutes, nil
}

func upstreamOriginURL(upstream ir.Upstream) string {
	u := &url.URL{
		Scheme: upstream.Scheme,
		Host: net.JoinHostPort(
			upstream.Host,
			strconv.Itoa(upstream.Port),
		),
	}

	return u.String()
}

func headersPredicate(headers map[string][]string) ([]snapshot.HeaderPredicate, error) {
	if len(headers) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(headers))
	for name := range headers {
		if name == "" {
			return nil, fmt.Errorf("header name cannot be empty")
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)

	predicates := make([]snapshot.HeaderPredicate, 0, len(keys))

	for _, name := range keys {
		values := headers[name]
		if len(values) == 0 {
			predicates = append(predicates, snapshot.HeaderPredicate{
				Name:          name,
				AllowedValues: nil,
			})
			continue
		}

		uniq := make(map[string]struct{}, len(values))
		clean := make([]string, 0, len(values))

		for _, value := range values {
			if value == "" {
				return nil, fmt.Errorf("header %q contains empty allowed value", name)
			}

			if _, ok := uniq[value]; ok {
				continue
			}

			uniq[value] = struct{}{}
			clean = append(clean, value)
		}

		predicates = append(predicates, snapshot.HeaderPredicate{
			Name:          name,
			AllowedValues: clean,
		})
	}

	return predicates, nil
}
