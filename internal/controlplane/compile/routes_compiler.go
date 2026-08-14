package compile

import (
	"fmt"
	"sort"

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
func Routes(serviceIDs map[string]snapshot.ServiceID, routes []ir.Route, policies *ir.Policies) ([]snapshot.CompiledRoute, error) {
	compiledRoutes := make([]snapshot.CompiledRoute, 0, len(routes))

	for _, route := range routes {
		// Path prefix is taken as-is, assuming it has already been normalized
		// in the validation/normalization representation of the control plane.
		pathPrefix := route.Match.PathPrefix

		serviceID, ok := serviceIDs[route.Service]
		if !ok {
			return nil, fmt.Errorf("route %q references unknown service %q", route.Name, route.Service)
		}

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

		// Compile route policy references into route-local executable plans.
		// The dataplane reads CompiledRoute.Headers directly and never resolves
		// policy names or IDs on the request path.
		headers, err := compileRoutePolicyHeaders(route.Policies, policies)
		if err != nil {
			return nil, fmt.Errorf("route %q: compile policy headers: %w", route.Name, err)
		}

		compiledRoutes = append(compiledRoutes, snapshot.CompiledRoute{
			Name:    route.Name,
			Service: serviceID,
			Match: snapshot.CompiledMatch{
				PathPrefix: pathPrefix,
				Methods:    methodMask,
				Headers:    headerPredicates,
			},

			Headers: headers,
		})
	}

	return compiledRoutes, nil
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

func compileRoutePolicyHeaders(
	refs []ir.PolicyRef,
	policies *ir.Policies,
) (snapshot.CompiledHeaders, error) {
	if len(refs) == 0 {
		return snapshot.CompiledHeaders{}, nil
	}
	if policies == nil {
		return snapshot.CompiledHeaders{}, fmt.Errorf("policy references require normalized policies")
	}

	merged := ir.Headers{
		Request: ir.HeadersOps{
			Add: make(map[string]string),
			Set: make(map[string]string),
		},
		Response: ir.HeadersOps{
			Add: make(map[string]string),
			Set: make(map[string]string),
		},
	}

	for _, ref := range refs {
		policy, ok := policies.Headers[ref.Name]
		if !ok {
			return snapshot.CompiledHeaders{}, fmt.Errorf("unknown headers policy %q", ref.Name)
		}

		if err := mergeHeadersOps(&merged.Request, &policy.Request); err != nil {
			return snapshot.CompiledHeaders{}, fmt.Errorf("policy %q request: %w", ref.Name, err)
		}
		if err := mergeHeadersOps(&merged.Response, &policy.Response); err != nil {
			return snapshot.CompiledHeaders{}, fmt.Errorf("policy %q response: %w", ref.Name, err)
		}
	}

	builder := newHeaderValueBuilder(estimateHeadersSize(&merged))
	return compileRouteHeaders(&merged, builder)
}

func mergeHeadersOps(dst *ir.HeadersOps, src *ir.HeadersOps) error {
	if dst == nil || src == nil {
		return nil
	}
	if dst.Add == nil {
		dst.Add = make(map[string]string)
	}
	if dst.Set == nil {
		dst.Set = make(map[string]string)
	}

	for _, name := range src.Remove {
		if headersOpContains(*dst, name) {
			return fmt.Errorf("header %q has conflicting operations", name)
		}
		dst.Remove = append(dst.Remove, name)
	}

	for _, name := range sortedStringMapKeys(src.Set) {
		if headersOpContains(*dst, name) {
			return fmt.Errorf("header %q has conflicting operations", name)
		}
		dst.Set[name] = src.Set[name]
	}

	for _, name := range sortedStringMapKeys(src.Add) {
		if headersOpContains(*dst, name) {
			return fmt.Errorf("header %q has conflicting operations", name)
		}
		dst.Add[name] = src.Add[name]
	}

	return nil
}

func headersOpContains(ops ir.HeadersOps, name string) bool {
	if _, ok := ops.Add[name]; ok {
		return true
	}
	if _, ok := ops.Set[name]; ok {
		return true
	}
	for _, removed := range ops.Remove {
		if removed == name {
			return true
		}
	}
	return false
}

func estimateHeadersSize(headers *ir.Headers) int {
	if headers == nil {
		return 0
	}

	var total int
	for _, value := range headers.Request.Set {
		total += len(value)
	}
	for _, value := range headers.Request.Add {
		total += len(value)
	}
	for _, value := range headers.Response.Set {
		total += len(value)
	}
	for _, value := range headers.Response.Add {
		total += len(value)
	}
	return total
}
