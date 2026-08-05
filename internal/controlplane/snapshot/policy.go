package snapshot

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

	// hop-by-hop headers
	HeaderConnection
	HeaderKeepAlive
	HeaderProxyAuthenticate
	HeaderProxyAuthorization
	HeaderTE
	HeaderTrailer
	HeaderTransferEncoding
	HeaderUpgrade

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

	// Offset inside immutable values blob.
	ValueOffset uint32

	// Value length inside values blob.
	ValueLength uint16
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

	// Immutable packed values blob.
	//
	// HeaderOp.ValueOffset and HeaderOp.ValueLength
	// reference slices inside this buffer.
	Values []byte
}

// CompiledHeaders contains request/response plans.
type CompiledHeaders struct {
	Request  CompiledHeadersPlan
	Response CompiledHeadersPlan
}

// HeaderRegistry resolves HeaderID into canonical header names.
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

type CompiledPolicies struct {
	Headers []CompiledHeaders
}
