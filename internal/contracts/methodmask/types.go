package methodmask

// MethodMask is a compact bitset representing allowed HTTP methods.
//
// Each bit corresponds to a specific HTTP method.
// This design enables fast branchless checks in the dataplane:
//
//	if route Methods & MethodGET != 0 { ... }
//
// Constraints:
//   - limited to 64 distinct methods (uint64 backing type)
//   - must be fully constructed at compile time (control plane)
type MethodMask uint64

const (
	MethodGET MethodMask = 1 << iota
	MethodPOST
	MethodPUT
	MethodDELETE
	MethodPATCH
	MethodOPTIONS
	MethodHEAD
)

// MethodAll represents a wildcard mask that allows all supported HTTP methods.
//
// This is equivalent to setting all known method bits.
// It is used when no method restrictions are defined in route configuration.
const MethodAll = MethodGET |
	MethodPOST |
	MethodPUT |
	MethodDELETE |
	MethodPATCH |
	MethodOPTIONS |
	MethodHEAD
