package snapshot

import "golang.org/x/time/rate"

// HeaderID is a compact runtime identifier.
//
// Static well-known headers are resolved during compilation
// into fixed numeric identifiers.
//
// Dynamic/custom headers are also assigned stable IDs
// inside the compiled configuration snapshot.
//
// Runtime must never perform:
//   - string normalization
//   - canonicalization
//   - map lookups by string
type HeaderID uint16

const (
	HeaderUnknown HeaderID = iota

	HeaderHost
	HeaderContentType
	HeaderContentLength

	HeaderAuthorization

	HeaderXForwardedFor
	HeaderXForwardedProto
	HeaderXRequestID

	HeaderServer

	HeaderXContentTypeOptions
	HeaderXFrameOptions
	HeaderXXSSProtection

	// HeaderDynamicStart Dynamic headers begin here.
	HeaderDynamicStart
)

// HeaderOpCode is a compiled executable operation.
//
// Runtime executor should behave like a tiny VM:
// sequential execution with predictable branches.
type HeaderOpCode uint8

const (
	// HeaderOpRemove Remove header unconditionally.
	HeaderOpRemove HeaderOpCode = iota

	// HeaderOpSet Set header unconditionally.
	HeaderOpSet

	// HeaderOpAddIfAbsent Add header only if absent.
	HeaderOpAddIfAbsent
)

// HeaderInstruction is a compact immutable runtime instruction.
//
// Layout is intentionally cache-friendly.
//
// Value fields are used only for Set/AddIfAbsent.
//
// ValueOffset/ValueLength reference bytes inside
// CompiledHeadersPlan.Values blob.
type HeaderInstruction struct {
	HeaderID HeaderID
	Op       HeaderOpCode

	// Value is precomputed at compile time and never mutated afterward,
	// so it's safe to share across every request and every snapshot swap.
	// There is no runtime string(...) conversion left to pay for.
	Value string
}

// CompiledHeadersPlan is a fully normalized executable plan.
//
// Compiler responsibilities:
//   - validation
//   - deduplication
//   - canonicalization
//   - conflict detection
//   - operation ordering
//   - value blob packing
//
// Runtime responsibilities:
//   - sequential execution only
//
// Execution order is guaranteed:
//
//  1. Remove
//  2. Set
//  3. AddIfAbsent
//
// Runtime must never sort or resolve conflicts.
type CompiledHeadersPlan struct {
	Ops []HeaderInstruction
}

// CompiledHeaders contains request/response plans.
type CompiledHeaders struct {
	Request  CompiledHeadersPlan
	Response CompiledHeadersPlan
}

// HeaderRegistry resolves HeaderID into canoniƒcal header names.
//
// Registry is immutable after compilation.
//
// Runtime should use direct indexed lookup:
//
//	name := registry.Names[id]
//
// No maps on hot path.
type HeaderRegistry struct {
	Names [][]byte
}

type CompiledRateLimit struct {
	Limit rate.Limit
	Burst int
}

type CompiledPolicies struct {
	Headers    []CompiledHeaders
	RateLimits []CompiledRateLimit
}
