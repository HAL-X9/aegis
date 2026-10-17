// Package yamlx decodes YAML documents from files in strict mode. It is
// shared by the process configuration (internal/config) and the routes
// manifest loader (internal/controlplane/loader), which otherwise have no
// reason to depend on one another.
package yamlx

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// DecodeFile reads the YAML file at path and decodes it into a new T.
//
// Decoding is strict: a key with no corresponding field in T is an error
// (yaml.v3 KnownFields). Only structural decoding is performed; semantic
// validation is left to the caller.
func DecodeFile[T any](path string) (*T, error) {
	var zero T
	return DecodeFileWithDefaults(path, zero)
}

// DecodeFileWithDefaults is DecodeFile with a pre-populated starting value:
// a field absent from the document keeps its value from defaults, and a field
// present in it, including an explicit zero, overwrites that value. This
// expresses a setting whose default is not the zero value (a feature that is
// on unless turned off, for instance) without pointer fields.
func DecodeFileWithDefaults[T any](path string, defaults T) (*T, error) {
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
