package compile

import (
	"fmt"
	"sort"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/snapshot"
	"golang.org/x/time/rate"
)

// Policies compiles the normalized IR rate-limit policies into a snapshot
// ready for the data-plane hot path.
//
// Named header policies are intentionally not compiled here. A route
// references header policies by name only so they can be merged inline
// onto that route's own CompiledRoute.Policies.Headers (see
// compileRoutePolicyHeaders in routes_compiler.go); nothing on the hot
// path looks a header plan up by policy name, so a second, policy-keyed
// copy is never built. See snapshot.CompiledPolicies's doc comment.
func Policies(policies *ir.Policies) (*snapshot.CompiledPolicies, error) {
	if policies == nil {
		return nil, fmt.Errorf("compile policies configuration: config is nil")
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
		RateLimits: compiledRateLimits,
	}, nil
}

// compileRouteHeaders compiles a route's merged request/response header
// mutation rules into snapshot form, resolving each header name to an ID
// through headerIDs (see headers_registry.go).
func compileRouteHeaders(headerIDs *HeaderRegistryBuilder, headers *ir.Headers) snapshot.CompiledHeaders {
	if headers == nil {
		return snapshot.CompiledHeaders{}
	}

	return snapshot.CompiledHeaders{
		Request:  snapshot.CompiledHeadersPlan{Ops: compileHeaderOps(headerIDs, &headers.Request)},
		Response: snapshot.CompiledHeadersPlan{Ops: compileHeaderOps(headerIDs, &headers.Response)},
	}
}

// compileHeaderOps transforms normalized operations into compact
// instructions, in the fixed remove/set/add-if-absent order that
// snapshot.CompiledHeadersPlan documents.
func compileHeaderOps(headerIDs *HeaderRegistryBuilder, ops *ir.HeadersOps) []snapshot.HeaderInstruction {
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
			HeaderID: headerIDs.resolve(name),
			Op:       snapshot.HeaderOpRemove,
		})
	}
	for _, name := range sortedStringMapKeys(ops.Set) {
		instructions = append(instructions, snapshot.HeaderInstruction{
			HeaderID: headerIDs.resolve(name),
			Op:       snapshot.HeaderOpSet,
			Value:    ops.Set[name],
		})
	}
	for _, name := range sortedStringMapKeys(ops.Add) {
		instructions = append(instructions, snapshot.HeaderInstruction{
			HeaderID: headerIDs.resolve(name),
			Op:       snapshot.HeaderOpAddIfAbsent,
			Value:    ops.Add[name],
		})
	}

	return instructions
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
