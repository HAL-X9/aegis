package ir

// NormalizedConfig represents a fully validated and canonicalized control
// plane configuration.
//
// The structure is produced after schema validation and normalization phases
// and serves as the primary intermediate representation passed into compiler
// stages.
//
// All contained entities are guaranteed to be semantically valid,
// deterministic, and normalized into runtime-independent form.
type NormalizedConfig struct {
	// NormalizedRoutes contains canonical route definitions prepared for
	// compilation into runtime routing structures.
	NormalizedRoutes []Route

	// NormalizedPolicies contains canonical reusable policy definitions
	// prepared for compilation into executable policy runtime structures.
	NormalizedPolicies NormalizedPolicies
}
