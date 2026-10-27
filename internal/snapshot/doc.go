// Package snapshot defines the immutable runtime representation of a gateway
// configuration.
//
// A CompiledConfig is produced by the control plane and consumed by the
// dataplane as read-only state. It contains compiled services, routes, policy
// tables, the header registry, and the flattened route index. The package
// carries no behavior beyond data access and must not be mutated after
// publication.

package snapshot
