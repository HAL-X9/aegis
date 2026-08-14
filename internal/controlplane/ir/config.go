package ir

// Config represents a fully validated and canonicalized control
// plane configuration.
//
// The structure is produced after schema validation and normalization phases
// and serves as the primary intermediate representation passed into compiler
// stages.
//
// All contained entities are guaranteed to be semantically valid,
// deterministic, and normalized into runtime-independent form.
type Config struct {
	// Services contains canonical service definitions prepared for
	// compilation into runtime service structures.
	Services Services

	// Routes contains canonical route definitions prepared for
	// compilation into runtime routing structures.
	Routes []Route

	// Policies contains canonical reusable policy definitions
	// prepared for compilation into executable policy runtime structures.
	Policies Policies
}
