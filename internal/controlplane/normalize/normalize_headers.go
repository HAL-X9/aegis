package normalize

import (
	"fmt"

	"net/textproto"

	"github.com/aegis/internal/controlplane/schema"
)

type NormalizedHeaders struct {
	Request  NormalizedHeadersOps
	Response NormalizedHeadersOps
}

type NormalizedHeadersOps struct {
	Add    map[string]string
	Set    map[string]string
	Remove []string
}

func normalizeHeadersOps(headersOps *schema.HeadersOps) (*NormalizedHeadersOps, error) {
	if headersOps == nil {
		return nil, fmt.Errorf("headers operations must not be nil")
	}

	out := &NormalizedHeadersOps{
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
