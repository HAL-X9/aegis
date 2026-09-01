package ir

// Services represents the canonical collection of service definitions within
// the normalized intermediate representation.
//
// Each service is uniquely identified by its configuration key.
type Services map[string]Service

// Service represents a canonical service definition within the normalized
// intermediate representation.
//
// A Service defines the upstream target configuration used for request
// forwarding.
type Service struct {
	// Upstream defines the normalized upstream destination configuration.
	Upstream Upstream
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
