// Package config defines the process-local app settings document (listeners,
// timeouts, logging). It is distinct from gateway routing and control-plane policy data.
package config

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

// UpstreamTransport configures outbound HTTP transport behavior including connection pooling limits and network timeouts for upstream service communication.
type UpstreamTransport struct {
	MaxIdleConns          int           `yaml:"max_idle_conns"`
	MaxIdleConnsPerHost   int           `yaml:"max_idle_conns_per_host"`
	MaxConnsPerHost       int           `yaml:"max_conns_per_host"`
	DialTimeout           time.Duration `yaml:"dial_timeout"`
	TLSHandshakeTimeout   time.Duration `yaml:"tls_handshake_timeout"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`
	IdleConnTimeout       time.Duration `yaml:"idle_conn_timeout"`
}

// Observability defines process-level telemetry configuration, primarily structured logging, for operational visibility and diagnostics.
type Observability struct {
	Logging Logging `yaml:"logging"`
	Tracing Tracing `yaml:"tracing"`
}

// Logging selects structured logger level and encoding for process output.
type Logging struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Tracing configures distributed tracing settings, including enablement and collector endpoint for exporting trace data.
type Tracing struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint"`
}

// Listeners separates public client traffic from internal system endpoints.
type Listeners struct {
	Public HTTP `yaml:"public"`
	System HTTP `yaml:"system"`
}

// Runtime is the unmarshaled root of the app YAML document; field tags match on-disk layout.
type Runtime struct {
	Listeners         Listeners         `yaml:"listeners"`
	UpstreamTransport UpstreamTransport `yaml:"upstream_transport"`
	Observability     Observability     `yaml:"observability"`
}
