package snapshot

// CompiledConfig is an immutable snapshot of all compiled routing rules.
//
// It is produced by the control plane and loaded into the dataplane as a
// read-only structure for request evaluation.
//
// Key property:
//   - MUST NOT be mutated after initialization
type CompiledConfig struct {
	// Routes is an ordered list of compiled routing rules.
	// Order may define priority (first match wins depending on router implementation).
	Routes []CompiledRoute

	Policies CompiledPolicies
}
