package normalize

import (
	"fmt"

	"net/textproto"

	"github.com/aegis/internal/controlplane/model"
)

type NormalizedHeaders struct {
	Add    map[string]string
	Set    map[string]string
	Remove []string
}

func NormalizeHeaders(headersOps *model.HeadersOps) (*NormalizedHeaders, error) {
	if headersOps == nil {
		return nil, fmt.Errorf("headers operations must not be nil")
	}

	out := &NormalizedHeaders{
		Add:    make(map[string]string, len(headersOps.Add)),
		Set:    make(map[string]string, len(headersOps.Set)),
		Remove: make([]string, len(headersOps.Remove)),
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
