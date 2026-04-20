// Package runtimeconfig defines the process-local runtime settings document (listeners,
// timeouts, logging). It is distinct from gateway routing and control-plane policy data.
package runtimeconfig

import (
	"crypto/tls"
	"time"
)

// HTTP configures the inbound HTTP server: listen address, optional TLS, limits, and timeouts.
type HTTP struct {
	Addr           string      `yaml:"addr"`
	TLS            *tls.Config `yaml:"tls"`
	Timeouts       Timeouts    `yaml:"timeouts"`
	MaxHeaderBytes int         `yaml:"max_header_bytes"`
}

// Timeouts sets net/http Server deadline fields; zero values use library defaults where applicable.
type Timeouts struct {
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
}

// Logging selects structured logger level and encoding for process output.
type Logging struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Runtime is the unmarshaled root of the runtime YAML document; field tags match on-disk layout.
type Runtime struct {
	HTTP    HTTP    `yaml:"http"`
	Logging Logging `yaml:"logging"`
}
