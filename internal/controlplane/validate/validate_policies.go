package validate

import (
	"fmt"
	"strings"

	"github.com/aegis/internal/controlplane/schema"
	"golang.org/x/net/http/httpguts"
)

func validatePolicies(policy *schema.Policies) error {
	if policy == nil {
		return fmt.Errorf("policies configuration must not be nil")
	}

	// Policy names are the keys of the headers map. Uniqueness is therefore
	// guaranteed by the YAML decoder (duplicate keys collapse into one entry);
	// here we only reject empty names, which the spec disallows.
	for policyName, headers := range policy.Headers {
		if policyName == "" {
			return fmt.Errorf("policy name under policies.headers must not be empty")
		}

		if err := validateHeaders(policyName, &headers); err != nil {
			return fmt.Errorf("headers validation failed in policy %q: %w", policyName, err)
		}
	}

	return nil
}

func validateHeaders(policyName string, headers *schema.Headers) error {
	if headers == nil {
		return fmt.Errorf("headers configuration must not be nil")
	}

	if err := validateHeadersOps("request", &headers.Request); err != nil {
		return fmt.Errorf("policy %q request invalid: %w", policyName, err)

	}

	if err := validateHeadersOps("response", &headers.Response); err != nil {
		return fmt.Errorf("policy %q response invalid: %w", policyName, err)
	}
	return nil
}

func validateHeadersOps(_ string, headersOps *schema.HeadersOps) error {
	if headersOps == nil {
		return fmt.Errorf("header operations configuration must not be nil")
	}

	for headerName := range headersOps.Add {
		if err := validateHeaderName(headerName); err != nil {
			return fmt.Errorf("invalid header name in add operation: %w", err)
		}
	}

	for headerName := range headersOps.Set {
		if err := validateHeaderName(headerName); err != nil {
			return fmt.Errorf("invalid header name in set operation: %w", err)
		}
	}

	for _, headerName := range headersOps.Remove {
		if err := validateHeaderName(headerName); err != nil {
			return fmt.Errorf("invalid header name in remove operation: %w", err)
		}
	}

	if err := validateNoOpConflicts(headersOps); err != nil {
		return err
	}

	return nil
}

// validateNoOpConflicts enforces that, within a single traffic direction, a
// header name is referenced by at most one operation group (add, set, remove).
//
// Header names are compared case-insensitively, matching how HTTP header keys
// are canonicalized downstream, so "X-Frame-Options" and "x-frame-options" are
// treated as the same header. Detecting conflicts here keeps the downstream
// compiler free of ambiguous mutation plans (for example a header that is both
// removed and re-added in the same direction).
//
// Groups are scanned in a fixed order (add, set, remove) so the reported
// conflict is deterministic regardless of map iteration order.
func validateNoOpConflicts(headersOps *schema.HeadersOps) error {
	// owner maps a case-folded header name to the operation group that first
	// referenced it, enabling precise conflict diagnostics.
	owner := make(map[string]string, len(headersOps.Add)+len(headersOps.Set)+len(headersOps.Remove))

	claim := func(name, group string) error {
		key := strings.ToLower(name)

		prev, seen := owner[key]
		if !seen {
			owner[key] = group
			return nil
		}

		if prev == group {
			return fmt.Errorf("header %q is listed more than once in the %q operation", name, group)
		}

		return fmt.Errorf(
			"header %q is referenced by both the %q and %q operations; a header may belong to only one operation group per direction",
			name, prev, group,
		)
	}

	for headerName := range headersOps.Add {
		if err := claim(headerName, "add"); err != nil {
			return err
		}
	}

	for headerName := range headersOps.Set {
		if err := claim(headerName, "set"); err != nil {
			return err
		}
	}

	for _, headerName := range headersOps.Remove {
		if err := claim(headerName, "remove"); err != nil {
			return err
		}
	}

	return nil
}

func validateHeaderName(name string) error {
	if !httpguts.ValidHeaderFieldName(name) {
		return fmt.Errorf("invalid header name %q", name)
	}

	return nil
}
