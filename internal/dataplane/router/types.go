package router

import "time"

// CompiledRoute is an immutable route representation used during request matching.
type CompiledRoute struct {
	// PathPrefix defines a normalized path prefix used by the radix index.
	PathPrefix string
	// Upstream identifies the target upstream cluster for matched requests.
	Upstream string
	// Timeout specifies the per-request upstream timeout.
	Timeout time.Duration
	// Retries defines the maximum retry attempts for upstream failures.
	Retries int
}

// CompiledManifest contains all routes produced by the compile phase.
type CompiledManifest struct {
	Routes []CompiledRoute
}
