package compile

import (
	"fmt"
	"sort"

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

	policyNames := make([]string, 0, len(policies.Headers))
	for name := range policies.Headers {
		policyNames = append(policyNames, name)
	}
	sort.Strings(policyNames)

	compiledHeaders := make([]snapshot.CompiledHeaders, 0, len(policyNames))

	for _, name := range policyNames {
		compiled, err := compileRouteHeaders(new(policies.Headers[name]), builder)
		if err != nil {
			return nil, fmt.Errorf("compile headers policy %q: %w", name, err)
		}

		compiledHeaders = append(compiledHeaders, compiled)
	}

	return &snapshot.CompiledPolicies{
		Headers: compiledHeaders,
	}, nil
}

// resolveHeaderID maps standard HTTP header string names to their
// strongly-typed numeric IDs for O(1) evaluation in the runtime.
func resolveHeaderID(name string) (snapshot.HeaderID, error) {
	switch name {
	case "Host":
		return snapshot.HeaderHost, nil
	case "Content-Type":
		return snapshot.HeaderContentType, nil
	case "Content-Length":
		return snapshot.HeaderContentLength, nil
	case "Authorization":
		return snapshot.HeaderAuthorization, nil
	case "X-Forwarded-For":
		return snapshot.HeaderXForwardedFor, nil
	case "X-Forwarded-Proto":
		return snapshot.HeaderXForwardedProto, nil
	case "X-Request-ID", "X-Request-Id":
		return snapshot.HeaderXRequestID, nil
	case "Server":
		return snapshot.HeaderServer, nil
	case "X-Content-Type-Options":
		return snapshot.HeaderXContentTypeOptions, nil
	case "X-Frame-Options":
		return snapshot.HeaderXFrameOptions, nil
	case "X-XSS-Protection", "X-Xss-Protection":
		return snapshot.HeaderXXSSProtection, nil
	default:
		return snapshot.HeaderUnknown, fmt.Errorf("unsupported header %q", name)
	}
}

// headerValueBuilder accumulates all static header values into a single
// continuous byte slice to eliminate runtime memory allocations.
type headerValueBuilder struct {
	buf []byte
}

func newHeaderValueBuilder(estimatedSize int) *headerValueBuilder {
	return &headerValueBuilder{
		buf: make([]byte, 0, estimatedSize),
	}
}

func (b *headerValueBuilder) Append(value string) (offset uint32, length uint16) {
	offset = uint32(len(b.buf))
	b.buf = append(b.buf, value...)
	length = uint16(len(value))
	return offset, length
}

// compileRouteHeaders compiles request and response header mutation rules
// into a unified snapshot format.
func compileRouteHeaders(
	headers *ir.NormalizedHeaders,
	builder *headerValueBuilder,
) (snapshot.CompiledHeaders, error) {

	if headers == nil {
		return snapshot.CompiledHeaders{}, nil
	}

	requestOps, err := compileHeaderOps(&headers.Request, builder)
	if err != nil {
		return snapshot.CompiledHeaders{}, fmt.Errorf("compile request header operations: %w", err)
	}

	responseOps, err := compileHeaderOps(&headers.Response, builder)
	if err != nil {
		return snapshot.CompiledHeaders{}, fmt.Errorf("compile response header operations: %w", err)
	}

	return snapshot.CompiledHeaders{
		Request: snapshot.CompiledHeadersPlan{
			Ops:    requestOps,
			Values: builder.buf,
		},
		Response: snapshot.CompiledHeadersPlan{
			Ops:    responseOps,
			Values: builder.buf,
		},
	}, nil
}

// compileHeaderOps transforms normalized operations into compact instructions.
func compileHeaderOps(
	ops *ir.NormalizedHeadersOps,
	builder *headerValueBuilder,
) ([]snapshot.HeaderInstruction, error) {

	if ops == nil {
		return nil, nil
	}

	estimatedOps := len(ops.Remove) + len(ops.Set) + len(ops.Add)
	if estimatedOps == 0 {
		return nil, nil
	}

	instructions := make([]snapshot.HeaderInstruction, 0, estimatedOps)

	for _, name := range ops.Remove {
		headerID, err := resolveHeaderID(name)
		if err != nil {
			return nil, err
		}

		instructions = append(instructions, snapshot.HeaderInstruction{
			HeaderID: headerID,
			Op:       snapshot.HeaderOpRemove,
		})
	}

	setNames := sortedStringMapKeys(ops.Set)
	for _, name := range setNames {
		headerID, err := resolveHeaderID(name)
		if err != nil {
			return nil, err
		}

		offset, length := builder.Append(ops.Set[name])

		instructions = append(instructions, snapshot.HeaderInstruction{
			HeaderID:    headerID,
			Op:          snapshot.HeaderOpSet,
			ValueOffset: offset,
			ValueLength: length,
		})
	}

	addNames := sortedStringMapKeys(ops.Add)
	for _, name := range addNames {
		headerID, err := resolveHeaderID(name)
		if err != nil {
			return nil, err
		}

		offset, length := builder.Append(ops.Add[name])

		instructions = append(instructions, snapshot.HeaderInstruction{
			HeaderID:    headerID,
			Op:          snapshot.HeaderOpAddIfAbsent,
			ValueOffset: offset,
			ValueLength: length,
		})
	}

	return instructions, nil
}

func sortedStringMapKeys(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// estimateStringsSize calculates total bytes for header values.
func estimateStringsSize(policies *ir.NormalizedPolicies) int {
	if policies == nil {
		return 0
	}

	var total int
	for _, h := range policies.Headers {
		for _, v := range h.Request.Set {
			total += len(v)
		}
		for _, v := range h.Request.Add {
			total += len(v)
		}
		for _, v := range h.Response.Set {
			total += len(v)
		}
		for _, v := range h.Response.Add {
			total += len(v)
		}
	}

	return total
}
