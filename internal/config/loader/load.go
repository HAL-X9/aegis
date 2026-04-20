package loader

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ReadAndDecodeYaml reads path and unmarshals its entire contents into a new T using YAML.
// It performs no validation beyond syntax; callers must validate semantic invariants.
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
