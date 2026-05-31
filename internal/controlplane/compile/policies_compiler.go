package compile

import (
	"fmt"

	"github.com/aegis/internal/controlplane/ir"
	"github.com/aegis/internal/controlplane/snapshot"
)

// Policy compiles the normalized IR policies into a highly optimized,
// allocation-free snapshot ready for the data plane hot-path.
func Policy(policies *ir.NormalizedPolicies) (*snapshot.CompiledPolicies, error) {
	if policies == nil {
		return nil, fmt.Errorf("compile policies configuration: config is nil")
	}

	estimatedSize := estimateStringsSize(policies)
	builder := newHeaderValueBuilder(estimatedSize)

	compiledHeaders := make([]snapshot.CompiledHeaders, 0, len(policies.Headers))

	for _, policy := range policies.Headers {
		compiled := compileRouteHeaders(&policy, builder)
		compiledHeaders = append(compiledHeaders, compiled)
	}

	return &snapshot.CompiledPolicies{
		Headers: compiledHeaders,
	}, nil
}

// resolveHeaderID maps standard HTTP header string names to their
// strongly-typed numeric IDs for O(1) evaluation in the runtime.
func resolveHeaderID(name string) snapshot.HeaderID {
	switch name {
	case "Host":
		return snapshot.HeaderHost
	case "Content-Type":
		return snapshot.HeaderContentType
	case "Content-Length":
		return snapshot.HeaderContentLength
	case "Authorization":
		return snapshot.HeaderAuthorization
	case "X-Forwarded-For":
		return snapshot.HeaderXForwardedFor
	case "X-Forwarded-Proto":
		return snapshot.HeaderXForwardedProto
	case "X-Request-ID":
		return snapshot.HeaderXRequestID
	case "Server":
		return snapshot.HeaderServer
	case "X-Content-Type-Options":
		return snapshot.HeaderXContentTypeOptions
	case "X-Frame-Options":
		return snapshot.HeaderXFrameOptions
	case "X-XSS-Protection":
		return snapshot.HeaderXXSSProtection
	default:
		return snapshot.HeaderUnknown
	}
}

// headerValueBuilder accumulates all static header values into a single
// continuous byte slice to eliminate runtime memory allocations.
type headerValueBuilder struct {
	buf []byte
}

// newHeaderValueBuilder initializes the builder with a pre-allocated capacity
// to prevent dynamic buffer resizing during compilation.
func newHeaderValueBuilder(estimatedSize int) *headerValueBuilder {
	return &headerValueBuilder{
		buf: make([]byte, 0, estimatedSize),
	}
}

// Append writes the string value to the shared buffer and returns
// its absolute memory coordinates (offset and length).
func (b *headerValueBuilder) Append(value string) (offset uint32, length uint16) {
	offset = uint32(len(b.buf))
	b.buf = append(b.buf, value...)
	length = uint16(len(value))
	return offset, length
}

// compileRouteHeaders compiles both request and response header mutation rules
// for a single route into a unified snapshot format, sharing the same underlying value buffer.
func compileRouteHeaders(
	headers *ir.NormalizedHeaders,
	builder *headerValueBuilder,
) snapshot.CompiledHeaders {
	if headers == nil {
		return snapshot.CompiledHeaders{}
	}

	return snapshot.CompiledHeaders{
		Request: snapshot.CompiledHeadersPlan{
			Ops:    compileHeaderOps(&headers.Request, builder),
			Values: builder.buf,
		},
		Response: snapshot.CompiledHeadersPlan{
			Ops:    compileHeaderOps(&headers.Response, builder),
			Values: builder.buf,
		},
	}
}

// compileHeaderOps transforms high-level normalized operations into a flat slice
// of compact binary instructions, enforcing strict semantic execution order:
// 1. Remove, 2. Set, 3. AddIfAbsent.
func compileHeaderOps(
	ops *ir.NormalizedHeadersOps,
	builder *headerValueBuilder,
) []snapshot.HeaderInstruction {
	if ops == nil {
		return nil
	}

	estimatedOps := len(ops.Remove) + len(ops.Set) + len(ops.Add)
	if estimatedOps == 0 {
		return nil
	}

	instructions := make([]snapshot.HeaderInstruction, 0, estimatedOps)

	for _, name := range ops.Remove {
		instructions = append(instructions, snapshot.HeaderInstruction{
			HeaderID: resolveHeaderID(name),
			Op:       snapshot.HeaderOpRemove,
		})
	}

	for name, value := range ops.Set {
		offset, length := builder.Append(value)
		instructions = append(instructions, snapshot.HeaderInstruction{
			HeaderID:    resolveHeaderID(name),
			Op:          snapshot.HeaderOpSet,
			ValueOffset: offset,
			ValueLength: length,
		})
	}

	for name, value := range ops.Add {
		offset, length := builder.Append(value)
		instructions = append(instructions, snapshot.HeaderInstruction{
			HeaderID:    resolveHeaderID(name),
			Op:          snapshot.HeaderOpAddIfAbsent,
			ValueOffset: offset,
			ValueLength: length,
		})
	}
	return instructions
}

// estimateStringsSize scans the IR policies to calculate the total byte length
// required for all static header values combined to optimize memory pre-allocation.
func estimateStringsSize(policies *ir.NormalizedPolicies) int {
	if policies == nil {
		return 0
	}

	var total int
	for _, h := range policies.Headers {
		for _, value := range h.Request.Set {
			total += len(value)
		}
		for _, value := range h.Request.Add {
			total += len(value)
		}

		for _, value := range h.Response.Set {
			total += len(value)
		}
		for _, value := range h.Response.Add {
			total += len(value)
		}
	}

	return total
}
