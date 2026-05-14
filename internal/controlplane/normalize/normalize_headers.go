package normalize

import (
	"fmt"

	"net/textproto"

	"github.com/aegis/internal/controlplane/schema"
)

// NormalizedHeaders represents a canonicalized form of header mutation operations.
// Header names are normalized to standard MIME canonical form to ensure consistent matching at runtime.
type NormalizedHeaders struct {
	Request  NormalizedHeadersOps
	Response NormalizedHeadersOps
}

// NormalizedHeadersOps defines a set of deterministic header mutation operations.
//
// Operations are applied in the following semantic order at runtime:
//  1. Remove
//  2. Set (overwrite existing values)
//  3. Add (append or insert values depending on execution model)
//
// All header keys are stored in canonical MIME format to ensure case-insensitive consistency.
type NormalizedHeadersOps struct {
	Add    map[string]string
	Set    map[string]string
	Remove []string
}

// normalizeHeadersOps converts schema-level header operations into a normalized internal representation.
//
// The function enforces header name canonicalization using MIME rules and guarantees deterministic output.
// It returns an error if the input schema is nil.
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
