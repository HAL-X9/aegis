package normalize

import (
	"fmt"

	"github.com/aegis/internal/controlplane/schema"
)

type NormalizedPolicies struct {
	Headers map[string]NormalizedHeaders
}

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
