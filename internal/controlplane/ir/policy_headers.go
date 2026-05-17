package ir

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
