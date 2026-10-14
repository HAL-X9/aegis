package normalize

import (
	"net/textproto"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

// Policies converts a validated policy source into a normalized intermediate
// representation: header names are canonicalized and rate limits are copied.
// It does not mutate its input.
func Policies(cfg *schema.Policies) *ir.Policies {
	out := &ir.Policies{
		Headers:    make(map[string]ir.Headers, len(cfg.Headers)),
		RateLimits: make(map[string]ir.RateLimit, len(cfg.RateLimits)),
	}

	for name, header := range cfg.Headers {
		out.Headers[name] = ir.Headers{
			Request:  normalizeHeadersOps(&header.Request),
			Response: normalizeHeadersOps(&header.Response),
		}
	}

	for rateLimitName, rl := range cfg.RateLimits {
		out.RateLimits[rateLimitName] = ir.RateLimit{Rate: rl.Rate, Burst: rl.Burst}
	}

	return out
}

// normalizeHeadersOps canonicalizes the header names of one direction's
// operations, producing deterministic output.
func normalizeHeadersOps(headersOps *schema.HeadersOps) ir.HeadersOps {
	out := ir.HeadersOps{
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

	return out
}
