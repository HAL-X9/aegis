package snapshot

// CompiledConfig is an immutable runtime snapshot produced by the control plane.
//
// It is consumed by the dataplane as read-only state and must not be mutated
// after publication.
type CompiledConfig struct {
	// Services contains compiled upstream service definitions addressable by name.
	Services CompiledServices

	// Routes is an ordered list of compiled routing rules.
	// Order may define priority (first match wins depending on router implementation).
	Routes []CompiledRoute

	// Policies contains reusable compiled policy plans.
	Policies CompiledPolicies

	// HeaderNames resolves dynamic HeaderIDs assigned during compilation.
	//
	// The registry is immutable after publication and is shared by the
	// routing engine for the lifetime of this compiled configuration.
	HeaderNames HeaderRegistry
}
