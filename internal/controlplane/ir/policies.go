package ir

// Policies represents a canonicalized policy configuration ready for compilation.
// It is an intermediate representation produced after source validation and normalization.
type Policies struct {
	Headers map[string]Headers
}
