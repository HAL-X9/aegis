package snapshot

import "github.com/aegis/internal/contracts/methodmask"

// CompiledRoute is the immutable runtime representation of a routing rule.
//
// It is produced by the control-plane compilation step and is consumed
// directly by the dataplane hot path.
//
// Design goals:
//   - zero dynamic parsing at runtime
//   - deterministic matching
//   - minimal allocations in request path
type CompiledRoute struct {
	Name string

	// Match contains all predicates required to determine route applicability.
	Match CompiledMatch

	// Upstream is a resolved backend target identifier (not a raw URL in ideal design).
	Upstream string
}

// CompiledMatch represents a fully normalized and optimized set of
// matching predicates for fast evaluation in the dataplane.
//
// All fields are expected to be preprocessed during compilation
// (no string parsing or regex compilation in runtime path).
type CompiledMatch struct {
	// PathPrefix is used for fast prefix matching in routing.
	// It is assumed to be normalized (e.g. leading slash ensured).
	PathPrefix string

	// Methods is a bitmask representing allowed HTTP methods.
	// Bitwise operations are used for O(1) evaluation in the dataplane.
	Methods methodmask.MethodMask

	// Headers is a simplified equality-based predicate.
	// NOTE: This is intentionally not a full expression system for performance reasons.
	Headers []HeaderPredicate
}

// HeaderPredicate defines a simple equality-based constraint on a single HTTP header.
//
// Design notes:
//   - intentionally NOT a full expression system (no regex, no AND/OR chains)
//   - optimized for fast equality checks in dataplane
//   - intended to be extended cautiously to avoid runtime complexity explosion
type HeaderPredicate struct {
	// Name is the HTTP header key (case-normalized during compilation).
	Name string

	// AllowedValues defines accepted values for this header.
	// Semantics:
	//   - len(AllowedValues) == 0: presence-only (header MUST exist, any value accepted)
	//   - len(AllowedValues) > 0: at least one request value MUST equal one of AllowedValues
	AllowedValues []string
}

// Decision represents the result of route evaluation in the dataplane.
//
// It is used to classify request routing outcomes without string allocations.
type Decision uint8

const (
	// DecisionRouteFound indicates a successful route match.
	DecisionRouteFound Decision = iota

	// DecisionMethodNotAllowed indicates route match but method mismatch.
	DecisionMethodNotAllowed

	// DecisionPathNotMatched indicates no route matched the request path.
	DecisionPathNotMatched
)
