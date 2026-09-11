package compile

import (
	"fmt"
	"sort"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
	"golang.org/x/time/rate"
)

// Policies compiles the normalized IR policies into a highly optimized,
// allocation-free snapshot ready for the data plane hot-path.
func Policies(policies *ir.Policies) (*snapshot.CompiledPolicies, error) {
	if policies == nil {
		return nil, fmt.Errorf("compile policies configuration: config is nil")
	}

	policyNames := make([]string, 0, len(policies.Headers))
	for name := range policies.Headers {
		policyNames = append(policyNames, name)
	}
	sort.Strings(policyNames)

	compiledHeaders := make([]snapshot.CompiledHeaders, 0, len(policyNames))
	for _, name := range policyNames {
		headers := policies.Headers[name]

		compiled, err := compileRouteHeaders(&headers)
		if err != nil {
			return nil, fmt.Errorf("compile headers policy %q: %w", name, err)
		}

		compiledHeaders = append(compiledHeaders, compiled)
	}

	names := sortedRateLimitNames(policies)
	compiledRateLimits := make([]snapshot.CompiledRateLimit, 0, len(names))
	for _, name := range names {
		rl := policies.RateLimits[name]
		compiledRateLimits = append(compiledRateLimits, snapshot.CompiledRateLimit{
			Limit: rate.Limit(rl.Rate),
			Burst: int(rl.Burst),
		})
	}

	return &snapshot.CompiledPolicies{
		Headers:    compiledHeaders,
		RateLimits: compiledRateLimits,
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

// compileRouteHeaders compiles request and response header mutation rules
// into a unified snapshot format. Header values are copied verbatim from
// the IR into the compiled instruction — there is no shared byte blob to
// build, so this takes no builder argument.
func compileRouteHeaders(headers *ir.Headers) (snapshot.CompiledHeaders, error) {
	if headers == nil {
		return snapshot.CompiledHeaders{}, nil
	}

	requestOps, err := compileHeaderOps(&headers.Request)
	if err != nil {
		return snapshot.CompiledHeaders{}, fmt.Errorf("compile request header operations: %w", err)
	}
	responseOps, err := compileHeaderOps(&headers.Response)
	if err != nil {
		return snapshot.CompiledHeaders{}, fmt.Errorf("compile response header operations: %w", err)
	}

	return snapshot.CompiledHeaders{
		Request:  snapshot.CompiledHeadersPlan{Ops: requestOps},
		Response: snapshot.CompiledHeadersPlan{Ops: responseOps},
	}, nil
}

// compileHeaderOps transforms normalized operations into compact instructions.
func compileHeaderOps(ops *ir.HeadersOps) ([]snapshot.HeaderInstruction, error) {
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

	for _, name := range sortedStringMapKeys(ops.Set) {
		headerID, err := resolveHeaderID(name)
		if err != nil {
			return nil, err
		}
		instructions = append(instructions, snapshot.HeaderInstruction{
			HeaderID: headerID,
			Op:       snapshot.HeaderOpSet,
			Value:    ops.Set[name],
		})
	}

	for _, name := range sortedStringMapKeys(ops.Add) {
		headerID, err := resolveHeaderID(name)
		if err != nil {
			return nil, err
		}
		instructions = append(instructions, snapshot.HeaderInstruction{
			HeaderID: headerID,
			Op:       snapshot.HeaderOpAddIfAbsent,
			Value:    ops.Add[name],
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

func sortedRateLimitNames(policies *ir.Policies) []string {
	if policies == nil {
		return nil
	}
	names := make([]string, 0, len(policies.RateLimits))
	for name := range policies.RateLimits {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
