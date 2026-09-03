package normalize

import (
	"fmt"
	"net/textproto"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

// Policies converts a raw policy source into a normalized intermediate representation.
//
// The function performs deterministic transformation of all policy definitions, including:
//   - validation of structural integrity
//   - normalization of header mutation rules
//   - canonicalization of all identifiers and header names
//
// The resulting structure is safe for compilation and runtime execution. The function does not mutate input state.
func Policies(cfg *schema.Policies) (*ir.Policies, error) {
	if cfg == nil {
		return nil, fmt.Errorf("normalize policies: config is nil")
	}

	out := &ir.Policies{
		Headers:    make(map[string]ir.Headers, len(cfg.Headers)),
		RateLimits: make(map[string]ir.RateLimit, len(cfg.RateLimits)),
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

		out.Headers[name] = ir.Headers{
			Request:  *req,
			Response: *resp,
		}
	}

	for rateLimitName, rl := range cfg.RateLimits {
		out.RateLimits[rateLimitName] = ir.RateLimit{Rate: rl.Rate, Burst: rl.Burst}
	}

	return out, nil
}

// normalizeHeadersOps converts source-level header operations into a normalized internal representation.
//
// The function enforces header name canonicalization using MIME rules and guarantees deterministic output.
// It returns an error if the input source is nil.
func normalizeHeadersOps(headersOps *schema.HeadersOps) (*ir.HeadersOps, error) {
	if headersOps == nil {
		return nil, fmt.Errorf("headers operations must not be nil")
	}

	out := &ir.HeadersOps{
		Add:    make(map[string]string, len(headersOps.Add)),
		Set:    make(map[string]string, len(headersOps.Set)),
		Remove: make([]string, 0, len(headersOps.Remove)),
	}

	for k, v := range headersOps.Add {
		nk := textproto.CanonicalMIMEHeaderKey(k)
		out.Add[nk] = v
	}

	for k, v := range headersOps.Set {
		nk := textproto.CanonicalMIMEHeaderKey(k)
		out.Set[nk] = v
	}

	for _, k := range headersOps.Remove {
		nk := textproto.CanonicalMIMEHeaderKey(k)
		out.Remove = append(out.Remove, nk)
	}

	return out, nil
}
