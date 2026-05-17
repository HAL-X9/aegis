package ir

// NormalizedPolicies represents a canonicalized policy configuration ready for compilation.
// It is an intermediate representation produced after source validation and normalization.
type NormalizedPolicies struct {
	Headers map[string]NormalizedHeaders
}
