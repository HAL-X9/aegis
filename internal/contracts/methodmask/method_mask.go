package methodmask

import "fmt"

// BuildMethodMask converts a list of HTTP method strings into a bitmask representation.
//
// The resulting MethodMask is used in the dataplane hot path to efficiently
// match incoming requests without string comparisons.
//
// Design notes:
//   - Empty input is treated as "match all methods" (MethodAll).
//   - Invalid method strings are treated as configuration errors and rejected.
//   - This function is intended to be used during configuration compilation
//     (control plane), not during request execution.
func BuildMethodMask(methods []string) (MethodMask, error) {
	// Empty method list means no restriction on HTTP methods.
	// This is equivalent to allowing all supported methods.
	if len(methods) == 0 {
		return MethodAll, nil
	}

	var mask MethodMask

	for _, method := range methods {
		bit, ok := MethodBit(method)
		if !ok {
			// Unknown methods are treated as configuration-time errors.
			// This prevents invalid routes from reaching the dataplane.
			return 0, fmt.Errorf("unsupported HTTP method: %s", method)
		}

		// Combine method bits using bitwise OR.
		// Each bit represents a distinct HTTP method.
		mask |= bit
	}

	return mask, nil
}

// MethodBit maps HTTP method strings to their corresponding bitmask value.
//
// This function is part of the compilation step and should not be used
// in the dataplane hot path.
//
// Returns:
//   - MethodMask bit for the given method
//   - false if the method is not supported
func MethodBit(method string) (MethodMask, bool) {
	switch method {
	case "GET":
		return MethodGET, true
	case "POST":
		return MethodPOST, true
	case "PUT":
		return MethodPUT, true
	case "DELETE":
		return MethodDELETE, true
	case "PATCH":
		return MethodPATCH, true
	case "OPTIONS":
		return MethodOPTIONS, true
	case "HEAD":
		return MethodHEAD, true
	default:
		// Unknown HTTP methods are rejected to ensure strict routing behavior
		// and prevent accidental wildcard matching.
		return 0, false
	}
}
