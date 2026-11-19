package schema

import "time"

// Services defines named backend services available to routes.
type Services map[string]Service

// Service defines a backend service referenced by routes.
type Service struct {
	Upstream Upstream `yaml:"upstream"`

	// Timeout bounds the whole upstream exchange, including retries and the
	// response body. Zero means no service-level timeout.
	Timeout time.Duration `yaml:"timeout"`

	Retries Retries `yaml:"retries"`
}

// Upstream names the endpoints that serve a service. Every target is reached
// with the same scheme; requests are spread across the targets in turn.
type Upstream struct {
	Scheme  string   `yaml:"scheme"`
	Targets []Target `yaml:"targets"`
}

// Target is one TCP endpoint of an upstream.
type Target struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// Retries configures upstream retries for a service.
type Retries struct {
	// Attempts is the total number of upstream attempts, including the first
	// one. 0 or 1 disables retries.
	Attempts int `yaml:"attempts"`

	// AttemptTimeout bounds each attempt, including its response body. Zero
	// means attempts are bounded only by Timeout.
	AttemptTimeout time.Duration `yaml:"attempt_timeout"`
}
