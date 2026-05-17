package schema

// Policies contains reusable policy definitions referenced by route entries.
// Keys in Headers are policy names and must be unique within the headers scope.
type Policies struct {
	Headers map[string]Headers `yaml:"headers"`
}
