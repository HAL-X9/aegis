package snapshot

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
// Value is used only for Set/AddIfAbsent.
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

// CompiledRateLimit is the runtime definition of one rate-limit policy, a
// token bucket: Limit is the sustained rate in requests per second and Burst
// the bucket size. A route selects its policy by index through
// CompiledRoutePolicies.RateLimitID.
type CompiledRateLimit struct {
	Limit float64
	Burst int
}

// CompiledRateLimits holds the compiled rate-limit definitions, addressed by
// CompiledRoutePolicies.RateLimitID.
//
// Header policies are not kept here: each route's header plan is merged from
// its referenced policies and stored in CompiledRoutePolicies.Headers.
type CompiledRateLimits struct {
	// Items stores the definitions in deterministic order, sorted by policy name.
	Items []CompiledRateLimit
}
