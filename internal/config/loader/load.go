package loader

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ReadAndDecodeYaml loads YAML from path and decodes it into T using yaml.v3 Decoder in strict mode.
// Unknown fields are treated as errors (KnownFields=true).
// Only structural decoding is performed; semantic validation is delegated to callers.
func ReadAndDecodeYaml[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var out T

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	if err = dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode config %s: %w", path, err)
	}

	return &out, nil
}
