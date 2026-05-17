package validate

import (
	"fmt"

	"github.com/aegis/internal/controlplane/schema"
	"golang.org/x/net/http/httpguts"
)

func validatePolicies(policy *schema.Policies) error {
	if policy == nil {
		return fmt.Errorf("policies configuration must not be nil")
	}

	for policyName, headers := range policy.Headers {
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

	return nil
}

func validateHeaderName(name string) error {
	if !httpguts.ValidHeaderFieldName(name) {
		return fmt.Errorf("invalid header name %q", name)
	}

	return nil
}
