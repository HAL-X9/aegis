package httpmethod

import "fmt"

// MaskOf converts a list of HTTP method names into a Mask.
//
// Design notes:
//   - An empty input is treated as "match all methods" (All).
//   - Unknown method names are configuration errors and are rejected.
//   - This function belongs to configuration compilation (control plane),
//     not to request execution.
func MaskOf(methods []string) (Mask, error) {
	if len(methods) == 0 {
		return All, nil
	}

	var mask Mask

	for _, method := range methods {
		bit, ok := Bit(method)
		if !ok {
			// Unknown methods are rejected so that invalid routes never reach
			// the dataplane.
			return 0, fmt.Errorf("unsupported HTTP method: %s", method)
		}

		mask |= bit
	}

	return mask, nil
}

// Bit maps an HTTP method name to its bit in a Mask.
//
// It reports false for any method that is not supported. Matching is
// case-sensitive, as HTTP method names are.
//
// Bit is used both by configuration validation and, on the request path, by
// the executor to classify the incoming method.
func Bit(method string) (Mask, bool) {
	switch method {
	case "GET":
		return GET, true
	case "POST":
		return POST, true
	case "PUT":
		return PUT, true
	case "DELETE":
		return DELETE, true
	case "PATCH":
		return PATCH, true
	case "OPTIONS":
		return OPTIONS, true
	case "HEAD":
		return HEAD, true
	default:
		return 0, false
	}
}
