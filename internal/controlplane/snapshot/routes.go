package snapshot

import "github.com/aegis/internal/contracts/methodmask"

// CompiledRoute is the immutable runtime representation of a routing rule.
type CompiledRoute struct {
	// Name is the stable route identifier used for diagnostics and observability.
	Name string

	// Service is the compiled service reference selected when the route matches.
	Service ServiceID

	// Match contains all predicates required to determine route applicability.
	Match CompiledMatch

	// Headers contains pre-compiled tokenized header mutation plans for the request and response paths.
	Headers CompiledHeaders
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
