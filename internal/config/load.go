package config

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
	var zero T
	return ReadAndDecodeYamlWithDefaults(path, zero)
}

// ReadAndDecodeYamlWithDefaults is ReadAndDecodeYaml with a pre-populated
// starting value: fields absent from the document keep their value from
// defaults, fields present in it overwrite them. This is how a setting whose
// default is not the zero value (such as a feature that is on unless turned
// off) is expressed without resorting to pointer fields.
func ReadAndDecodeYamlWithDefaults[T any](path string, defaults T) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	out := defaults

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	if err = dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode config %s: %w", path, err)
	}

	return &out, nil
}
