package schema

// Route binds request matching criteria to a single upstream. Name is a stable
// identifier for logs, metrics, and diagnostics.
type Route struct {
	Name     string   `yaml:"name"`
	Service  string   `yaml:"service"`
	Match    Match    `yaml:"match"`
	Policies []Policy `yaml:"policies"`
}

// Match selects inbound requests. Methods, when non-empty, restricts the HTTP verb set;
// when empty, any method matches PathPrefix.
type Match struct {
	PathPrefix string              `yaml:"path_prefix"`
	Methods    []string            `yaml:"methods"`
	Headers    map[string][]string `yaml:"headers"`
}

// Policy references a reusable policy by name.
type Policy struct {
	Name string `yaml:"name"`
}
