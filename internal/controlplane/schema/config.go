package schema

// GatewayConfig is the unmarshaled root of the gateway control-plane document.
// Field tags define the on-disk YAML layout; callers must validate before use.
type GatewayConfig struct {
	Routes   []Route  `yaml:"routes"`
	Policies Policies `yaml:"policies"`
}
