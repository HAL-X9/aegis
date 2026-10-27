// Package pipeline assembles the control-plane stages into a single build.
//
// Build takes a validated configuration, normalizes it, compiles services,
// routes, and policies, and produces an immutable snapshot.CompiledConfig.
// The input configuration is never mutated.

package pipeline
