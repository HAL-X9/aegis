package compile

import (
	"fmt"
	"slices"
	"sort"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

// Routes transforms normalized route definitions into immutable route
// entries optimized for dataplane execution.
//
// This is a control-plane operation and must NOT be used in the dataplane.
//
// headerIDs resolves every header name a route's policies mention to a
// snapshot.HeaderID; pass the same builder to every Routes call for one
// config build, then attach headerIDs.Registry() to
// snapshot.CompiledConfig.HeaderNames — see headers_registry.go.
//
// Key properties of the output:
//   - fully precomputed (no parsing in runtime path)
//   - deterministic routing rules
//   - optimized for fast lookup and evaluation
func Routes(headerIDs *HeaderRegistryBuilder, serviceIDs map[string]snapshot.ServiceID, routes []ir.Route, policies *ir.Policies) ([]snapshot.CompiledRoute, error) {
	compiledRoutes := make([]snapshot.CompiledRoute, 0, len(routes))

	for _, route := range routes {
		pathPrefix := route.Match.PathPrefix

		serviceID, ok := serviceIDs[route.Service]
		if !ok {
			return nil, fmt.Errorf("route %q references unknown service %q", route.Name, route.Service)
		}

		methodMask, err := methodmask.BuildMethodMask(route.Match.Methods)
		if err != nil {
			return nil, fmt.Errorf("route %q: compile method mask: %w", route.Name, err)
		}

		headerPredicates, err := headersPredicate(route.Match.Headers)
		if err != nil {
			return nil, fmt.Errorf("route %q: compile headers predicate: %w", route.Name, err)
		}

		if err = validatePolicyRefsExist(route.Policies, policies); err != nil {
			return nil, fmt.Errorf("route %q: %w", route.Name, err)
		}

		headers, err := compileRoutePolicyHeaders(headerIDs, route.Policies, policies)
		if err != nil {
			return nil, fmt.Errorf("route %q: compile policy headers: %w", route.Name, err)
		}

		rateLimitID, err := resolveRouteRateLimitID(route.Name, route.Policies, policies)
		if err != nil {
			return nil, fmt.Errorf("route %q: compile policy rate limit: %w", route.Name, err)
		}

		compiledRoutes = append(compiledRoutes, snapshot.CompiledRoute{
			Name:    route.Name,
			Service: serviceID,
			Match: snapshot.CompiledMatch{
				PathPrefix: pathPrefix,
				Methods:    methodMask,
				Headers:    headerPredicates,
			},
			Policies: snapshot.CompiledRoutePolicies{
				Headers:     headers,
				RateLimitID: rateLimitID,
			},
		})
	}

	return compiledRoutes, nil
}

// headersPredicate compiles a route's header *match* predicates (does the
// request carry this header, optionally with one of these values). This
// is unrelated to header *mutation* IDs (headerIDs above): a match
// predicate is evaluated against http.Header by name
// (internal/dataplane/router.HeadersMatch), never through a HeaderID, so
// it needs no registry.
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

func validatePolicyRefsExist(refs []ir.PolicyRef, policies *ir.Policies) error {
	if len(refs) == 0 {
		return nil
	}
	if policies == nil {
		return fmt.Errorf("policy references require normalized policies")
	}

	for _, ref := range refs {
		_, isHeaders := policies.Headers[ref.Name]
		_, isRateLimit := policies.RateLimits[ref.Name]

		if !isHeaders && !isRateLimit {
			return fmt.Errorf("references unknown policy %q", ref.Name)
		}
	}

	return nil
}

// compileRoutePolicyHeaders merges every headers-policy a route
// references into one IR headers set, then compiles it directly — no
// shared byte-blob builder is needed, since HeaderInstruction carries its
// Value inline and headerIDs resolves names to IDs as it goes.
func compileRoutePolicyHeaders(
	headerIDs *HeaderRegistryBuilder,
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
			continue
		}

		if err := mergeHeadersOps(&merged.Request, &policy.Request); err != nil {
			return snapshot.CompiledHeaders{}, fmt.Errorf("policy %q request: %w", ref.Name, err)
		}
		if err := mergeHeadersOps(&merged.Response, &policy.Response); err != nil {
			return snapshot.CompiledHeaders{}, fmt.Errorf("policy %q response: %w", ref.Name, err)
		}
	}

	return compileRouteHeaders(headerIDs, &merged), nil
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
	return slices.Contains(ops.Remove, name)
}

func resolveRouteRateLimitID(routeName string, refs []ir.PolicyRef, policies *ir.Policies) (int32, error) {
	names := sortedRateLimitNames(policies)
	index := make(map[string]int32, len(names))
	for i, n := range names {
		index[n] = int32(i)
	}

	found := int32(-1)
	for _, ref := range refs {
		id, ok := index[ref.Name]
		if !ok {
			continue
		}
		if found != -1 {
			return -1, fmt.Errorf("route %q references multiple rate-limit policies", routeName)
		}
		found = id
	}
	return found, nil
}
