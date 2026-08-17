package normalize

import (
	"net/http"
	"slices"
	"strings"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

// Routes converts validated source route definitions into a
// canonical intermediate representation suitable for compilation.
//
// The function assumes input has already passed validation. As a result,
// normalization focuses exclusively on deterministic canonicalization and
// allocation of immutable IR structures.
func Routes(routes []schema.Route) []ir.Route {
	if len(routes) == 0 {
		return nil
	}

	normalized := make([]ir.Route, 0, len(routes))

	for _, route := range routes {
		normalized = append(normalized, ir.Route{
			Name:    strings.TrimSpace(route.Name),
			Service: strings.TrimSpace(route.Service),
			Match: ir.Match{
				PathPrefix: normalizePathPrefix(route.Match.PathPrefix),
				Methods:    normalizeMethods(route.Match.Methods),
				Headers:    normalizeHeaders(route.Match.Headers),
			},
			Policies: normalizePolicyRefs(route.Policies),
		})
	}

	return normalized
}

// normalizePathPrefix canonicalizes route path prefixes.
func normalizePathPrefix(path string) string {
	path = strings.TrimSpace(path)

	if path == "/" {
		return path
	}

	return strings.TrimRight(path, "/")
}

// normalizeMethods canonicalizes HTTP method declarations into stable
// uppercase form and deterministic ordering.
func normalizeMethods(methods []string) []string {
	if len(methods) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(methods))

	for _, method := range methods {
		normalized = append(normalized, strings.ToUpper(method))
	}

	slices.Sort(normalized)

	return normalized
}

// normalizeHeaders canonicalizes header match configuration.
//
// Header names are converted into canonical MIME header form.
func normalizeHeaders(headers map[string][]string) map[string][]string {
	if len(headers) == 0 {
		return nil
	}

	normalized := make(map[string][]string, len(headers))

	for key, values := range headers {
		canonicalKey := http.CanonicalHeaderKey(key)

		normalizedValues := make([]string, 0, len(values))

		for _, value := range values {
			normalizedValues = append(normalizedValues, strings.TrimSpace(value))
		}

		normalized[canonicalKey] = normalizedValues
	}

	return normalized
}

func normalizePolicyRefs(policies []schema.Policy) []ir.PolicyRef {
	if len(policies) == 0 {
		return nil
	}

	out := make([]ir.PolicyRef, 0, len(policies))
	for _, policy := range policies {
		out = append(out, ir.PolicyRef{
			Name: strings.TrimSpace(policy.Name),
		})
	}

	return out
}
