package ir

// Services represents the canonical collection of service definitions within
// the normalized intermediate representation.
//
// Each service is uniquely identified by its configuration key.
type Services map[string]Service

// Service represents a canonical service definition within the normalized
// intermediate representation.
type Service struct {
	// Upstream defines the normalized upstream destination configuration.
	Upstream Upstream

	// Retries defines the normalized upstream retry configuration.
	Retries Retries
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

// Retries defines normalized retry settings. Attempts is always >= 1 and
// counts the first try (1 means no retries).
type Retries struct {
	Attempts int
}
