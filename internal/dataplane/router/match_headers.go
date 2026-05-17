package router

import (
	"net/http"

	"github.com/aegis/internal/controlplane/compile"
)

// HeadersMatch reports whether all header predicates are satisfied
// by the provided request headers.
//
// A predicate matches when:
//   - the header is present in the request;
//   - and, if AllowedValues is non-empty, at least one request value
//     equals one of the allowed values.
//
// An empty AllowedValues slice means that only header presence
// is required.
func HeadersMatch(preds []compile.HeaderPredicate, headers http.Header) bool {
	if len(preds) == 0 {
		return true
	}

	for _, pred := range preds {
		reqValues, ok := headers[pred.Name]
		if !ok {
			return false
		}

		if len(pred.AllowedValues) == 0 {
			continue
		}

		if !anyValueAllowed(reqValues, pred.AllowedValues) {
			return false
		}
	}

	return true
}

// anyValueAllowed reports whether any request header value matches
// any value from the allowed set.
func anyValueAllowed(reqValues []string, allowed []string) bool {
	for _, rv := range reqValues {
		for _, av := range allowed {
			if rv == av {
				return true
			}
		}
	}
	return false
}
