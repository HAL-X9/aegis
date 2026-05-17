package ir

// Route represents a canonical route definition within the normalized
// intermediate representation.
//
// A Route defines request matching rules, upstream forwarding configuration,
// and attached policy references prepared for subsequent compilation stages.
type Route struct {
	// Name uniquely identifies the route within the configuration scope.
	Name string

	// Match defines normalized request matching criteria.
	Match Match

	// Upstream defines the normalized upstream destination configuration.
	Upstream Upstream

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

// Upstream defines a normalized upstream target configuration used for
// request forwarding.
type Upstream struct {
	// Scheme defines the upstream transport scheme.
	Scheme string

	// Host defines the upstream hostname or network address.
	Host string

	// Port defines the upstream network port.
	Port int
}

// PolicyRef represents a normalized reference to a reusable policy
// definition.
type PolicyRef struct {
	// Name uniquely identifies the referenced policy.
	Name string
}
