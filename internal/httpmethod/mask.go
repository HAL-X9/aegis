// Package httpmethod defines the HTTP methods Aegis routes on and their
// compact bitmask representation.
//
// It is a leaf package with no dependencies on other internal packages, so
// the control plane, the compiled snapshot, and the data plane share one
// definition of the supported methods.
package httpmethod

// Mask is a compact bitset representing allowed HTTP methods.
//
// Each bit corresponds to a specific HTTP method.
// This design enables fast branchless checks in the dataplane:
//
//	if route Methods & GET != 0 { ... }
//
// Constraints:
//   - limited to 64 distinct methods (uint64 backing type)
//   - must be fully constructed at compile time (control plane)
type Mask uint64

const (
	GET Mask = 1 << iota
	POST
	PUT
	DELETE
	PATCH
	OPTIONS
	HEAD
)

// All represents a wildcard mask that allows all supported HTTP methods.
//
// It is used when no method restrictions are defined in route configuration.
const All = GET |
	POST |
	PUT |
	DELETE |
	PATCH |
	OPTIONS |
	HEAD
