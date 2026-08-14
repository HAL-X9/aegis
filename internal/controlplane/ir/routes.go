package ir

// Route represents a canonical route definition within the intermediate
// representation.
//
// A Route defines request matching rules, a target service reference, and
// attached policy references prepared for subsequent compilation stages.
type Route struct {
	// Name uniquely identifies the route within the configuration scope.
	Name string

	// Service identifies the normalized service targeted by the route.
	Service string

	// Match defines normalized request matching criteria.
	Match Match

	// Policies contains ordered policy references attached to the route.
	//
	// Declaration order is preserved to maintain deterministic policy execution
	// semantics during runtime compilation.
	Policies []PolicyRef
}

// Match defines canonical request matching criteria used during route
// resolution.
type Match struct {
	// PathPrefix defines the normalized request path prefix matcher.
	PathPrefix string

	// Methods contains normalized HTTP methods allowed for the route.
	//
	// Methods are stored in canonical uppercase form.
	Methods []string

	// Headers defines normalized request header matching constraints.
	//
	// Header names are stored in canonical MIME header form.
	Headers map[string][]string
}

// PolicyRef represents a normalized reference to a reusable policy
// definition.
type PolicyRef struct {
	// Name uniquely identifies the referenced policy.
	Name string
}
