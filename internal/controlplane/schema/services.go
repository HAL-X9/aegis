package schema

// Services defines named backend services available to routes.
type Services map[string]Service

// Service defines a backend service referenced by routes.
type Service struct {
	Upstream Upstream `yaml:"upstream"`
}

// Upstream names a TCP endpoint (scheme, host and port) for proxied traffic.
type Upstream struct {
	Scheme string `yaml:"scheme"`
	Host   string `yaml:"host"`
	Port   int    `yaml:"port"`
}
