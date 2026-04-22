package router

import "time"

type Decision uint8
type MethodMask uint64

// CompiledRoute is a normalized route used at match time.
type CompiledRoute struct {
	PathPrefix string
	MethodMask MethodMask
	Upstream   string
	Timeout    time.Duration
	Retries    int
}

type CompileManifest struct {
	Routes []CompiledRoute
}
