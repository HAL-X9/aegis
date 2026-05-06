package compiler

// CompiledPolicies contains immutable, precompiled policy artifacts used by the
// dataplane at runtime.
//
// Policies are indexed by logical policy name. Each entry is expected to be
// fully validated and normalized during compilation so runtime application can
// execute without additional schema checks.
type CompiledPolicies struct {
	Headers map[string]CompiledHeaders
}

// CompiledHeaders holds precompiled header mutation plans for both traffic
// directions.
//
// Request operations are applied to the upstream-bound request.
// Response operations are applied to the downstream-bound response.
type CompiledHeaders struct {
	Request  CompiledHeadersOps
	Response CompiledHeadersOps
}

// CompiledHeadersOps defines a normalized execution plan for header mutations
// in one traffic direction.
//
// All header names must be canonicalized and deduplicated at compile time.
// Operation ordering is deterministic and intended to be applied as:
//  1. Remove
//  2. Set
//  3. AddIfAbsent
//
// This ordering guarantees predictable behavior when policies are audited and
// replayed across environments.
type CompiledHeadersOps struct {
	// Remove lists header names to delete unconditionally.
	Remove []string

	// Set lists header assignments that overwrite existing values.
	Set []HeaderKV

	// AddIfAbsent lists header assignments applied only when the target header
	// key is currently missing.
	AddIfAbsent []HeaderKV
}

// HeaderKV is a normalized header assignment used by compiled mutation plans.
//
// Key must be a canonical HTTP header name.
// Value is the prevalidated literal assigned by the operation.
type HeaderKV struct {
	Key   string
	Value string
}
