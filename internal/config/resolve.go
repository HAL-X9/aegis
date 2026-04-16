package config

import (
	"fmt"
	"os"
)

func ResolvePath(cliPath, envKey string) (string, error) {
	if cliPath != "" {
		return cliPath, nil
	}

	if envKey == "" {
		return "", fmt.Errorf("invalid configuration: cliPath is empty and envKey is not provided")
	}

	value, ok := os.LookupEnv(envKey)
	if !ok {
		return "", fmt.Errorf("%s is not set and --config is empty", envKey)
	}
	if value == "" {
		return "", fmt.Errorf("%s is set but empty", envKey)
	}

	return value, nil
}
