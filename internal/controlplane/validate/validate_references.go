package validate

import (
	"fmt"
	"slices"
	"strings"

	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

// validateReferences checks the links between routes and the services and
// policies they name, and the header conflicts between the policies a route
// combines. Each rule here depends on more than one definition, so it cannot
// be checked by validating a single section alone.
func validateReferences(routes []schema.Route, services schema.Services, policies *schema.Policies) error {
	for i := range routes {
		if err := validateRouteReferences(&routes[i], services, policies); err != nil {
			return fmt.Errorf("invalid route at index %d: %w", i, err)
		}
	}

	return nil
}

func validateRouteReferences(route *schema.Route, services schema.Services, policies *schema.Policies) error {
	if _, ok := services[route.Service]; !ok {
		return fmt.Errorf("route validation failed: service %q is not defined", route.Service)
	}

	rateLimitRefs := 0
	for _, ref := range route.Policies {
		_, isHeaders := policies.Headers[ref.Name]
		_, isRateLimit := policies.RateLimits[ref.Name]

		switch {
		case isRateLimit:
			rateLimitRefs++
		case !isHeaders:
			return fmt.Errorf("route validation failed: references unknown policy %q", ref.Name)
		}
	}

	if rateLimitRefs > 1 {
		return fmt.Errorf("route validation failed: references multiple rate-limit policies")
	}

	return validateHeaderConflicts(route.Policies, policies)
}

// validateHeaderConflicts ensures that, within each direction, no header is
// changed by more than one headers policy a route references. Names are
// compared case-insensitively, as they are after canonicalization downstream.
func validateHeaderConflicts(refs []schema.Policy, policies *schema.Policies) error {
	requestOwners := make(map[string]string)
	responseOwners := make(map[string]string)

	for _, ref := range refs {
		headers, ok := policies.Headers[ref.Name]
		if !ok {
			continue
		}

		if err := claimHeaderNames(requestOwners, ref.Name, &headers.Request); err != nil {
			return fmt.Errorf("route validation failed: request: %w", err)
		}
		if err := claimHeaderNames(responseOwners, ref.Name, &headers.Response); err != nil {
			return fmt.Errorf("route validation failed: response: %w", err)
		}
	}

	return nil
}

// claimHeaderNames records every header that ops changes as owned by policy.
// It fails if a header is already owned, whether by another policy or by the
// same policy referenced twice.
func claimHeaderNames(owners map[string]string, policy string, ops *schema.HeadersOps) error {
	names := make([]string, 0, len(ops.Add)+len(ops.Set)+len(ops.Remove))
	for name := range ops.Add {
		names = append(names, name)
	}
	for name := range ops.Set {
		names = append(names, name)
	}
	names = append(names, ops.Remove...)
	slices.Sort(names)

	for _, name := range names {
		key := strings.ToLower(name)

		owner, seen := owners[key]
		switch {
		case !seen:
			owners[key] = policy
		case owner == policy:
			return fmt.Errorf("header %q is changed more than once by policy %q", name, policy)
		default:
			return fmt.Errorf("header %q is changed by both policy %q and policy %q", name, owner, policy)
		}
	}

	return nil
}
