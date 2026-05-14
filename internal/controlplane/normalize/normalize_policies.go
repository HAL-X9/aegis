package normalize

import (
	"fmt"

	"github.com/aegis/internal/controlplane/schema"
)

// NormalizedPolicies represents a canonicalized policy configuration ready for compilation.
// It is an intermediate representation produced after schema validation and normalization.
type NormalizedPolicies struct {
	Headers map[string]NormalizedHeaders
}

// Normalize converts a raw policy schema into a normalized intermediate representation.
//
// The function performs deterministic transformation of all policy definitions, including:
//   - validation of structural integrity
//   - normalization of header mutation rules
//   - canonicalization of all identifiers and header names
//
// The resulting structure is safe for compilation and runtime execution. The function does not mutate input state.
func Normalize(cfg *schema.Policies) (*NormalizedPolicies, error) {
	if cfg == nil {
		return nil, fmt.Errorf("")
	}

	out := &NormalizedPolicies{
		Headers: make(map[string]NormalizedHeaders, len(cfg.Headers)),
	}

	for name, header := range cfg.Headers {
		req, err := normalizeHeadersOps(&header.Request)
		if err != nil {
			return nil, fmt.Errorf(
				"normalize request headers policy %q: %w",
				name,
				err,
			)
		}

		resp, err := normalizeHeadersOps(&header.Response)
		if err != nil {
			return nil, fmt.Errorf(
				"normalize response headers policy %q: %w",
				name,
				err,
			)
		}

		out.Headers[name] = NormalizedHeaders{
			Request:  *req,
			Response: *resp,
		}
	}

	return out, nil
}
