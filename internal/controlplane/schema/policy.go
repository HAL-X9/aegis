package schema

// Policies contains reusable policy definitions referenced by route entries.
// Keys in Headers are policy names and must be unique within the headers scope.
type Policies struct {
	Headers map[string]Headers `yaml:"headers"`
}

// Headers defines header mutations applied to inbound and outbound traffic.
// Request rules apply before proxying to upstream. Response rules apply before
// writing the upstream response back to the client.
type Headers struct {
	Request  HeadersOps `yaml:"request"`
	Response HeadersOps `yaml:"response"`
}

// HeadersOps describes header mutation operations for a single traffic
// direction.
//
// Add sets a header only when it is currently absent.
// Set unconditionally overwrites the header value.
// Remove deletes header names listed in the slice.
//
// If the same header appears in multiple operation groups, implementations
// should reject the policy as invalid during validation.
type HeadersOps struct {
	Add    map[string]string `yaml:"add"`
	Set    map[string]string `yaml:"set"`
	Remove []string          `yaml:"remove"`
}
