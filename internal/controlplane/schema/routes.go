package schema

// Route binds request matching criteria to a single upstream. Name is a stable
// identifier for logs, metrics, and diagnostics.
type Route struct {
	Name     string   `yaml:"name"`
	Match    Match    `yaml:"match"`
	Upstream Upstream `yaml:"upstream"`
	Policies []Policy `yaml:"policies"`
}

// Match selects inbound requests. Methods, when non-empty, restricts the HTTP verb set;
// when empty, any method matches PathPrefix.
type Match struct {
	PathPrefix string              `yaml:"path_prefix"`
	Methods    []string            `yaml:"methods"`
	Headers    map[string][]string `yaml:"headers"`
}

// Upstream names a TCP endpoint (scheme, host and port) for proxied traffic.
type Upstream struct {
	Scheme string `yaml:"scheme"`
	Host   string `yaml:"host"`
	Port   int    `yaml:"port"`
}

type Policy struct {
	Name string `yaml:"name"`
}
