// Package loader resolves configuration file paths and reads YAML documents into typed values.
package loader

const (
	// EnvRuntimeConfigPath names the environment variable holding the process runtime config file path.
	EnvRuntimeConfigPath = "AEGIS_RUNTIME_CONFIG_PATH"
	// EnvRoutesConfigPath names the environment variable holding the gateway routes manifest path.
	EnvRoutesConfigPath = "AEGIS_ROUTES_CONFIG_PATH"
)
