package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func ReadAndDecodeYaml[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var out T
	if err = yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal config %s: %w", path, err)
	}

	return &out, nil
}
