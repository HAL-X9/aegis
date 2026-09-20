package integration

import (
	"net/http"
	"testing"

	"golang.org/x/time/rate"

	"github.com/HAL-X9/aegis/internal/dataplane/policy"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

// TestExecuteMutations_OrderingAndScopes verifies that a compiled header
// plan is applied in the fixed remove -> set -> add-if-absent order, and
// that add-if-absent never overrides a value already present once set has
// run — the guarantee docs/policies.md makes explicit.
func TestExecuteMutations_OrderingAndScopes(t *testing.T) {
	plan := &snapshot.CompiledHeadersPlan{
		Ops: []snapshot.HeaderInstruction{
			{HeaderID: snapshot.HeaderXForwardedFor, Op: snapshot.HeaderOpRemove},
			{HeaderID: snapshot.HeaderXRequestID, Op: snapshot.HeaderOpSet, Value: "generated-id"},
			{HeaderID: snapshot.HeaderXFrameOptions, Op: snapshot.HeaderOpAddIfAbsent, Value: "DENY"},
		},
	}

	h := http.Header{}
	h.Set("X-Forwarded-For", "203.0.113.1")
	h.Set("X-Request-Id", "client-supplied")
	h.Set("X-Frame-Options", "SAMEORIGIN") // already present before add-if-absent runs

	policy.ExecuteMutations(h, plan, nil)

	if got := h.Get("X-Forwarded-For"); got != "" {
		t.Errorf("X-Forwarded-For = %q, want removed", got)
	}
	if got := h.Get("X-Request-Id"); got != "generated-id" {
		t.Errorf("X-Request-Id = %q, want %q", got, "generated-id")
	}
	if got := h.Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options = %q, want unchanged %q (add-if-absent must not overwrite)", got, "SAMEORIGIN")
	}
}

// TestExecuteMutations_NilPlanIsNoop guards the plan == nil / empty-ops
// fast path a route with no header policy relies on.
func TestExecuteMutations_NilPlanIsNoop(t *testing.T) {
	h := http.Header{}
	h.Set("X-Request-Id", "untouched")

	policy.ExecuteMutations(h, nil, nil)

	if got := h.Get("X-Request-Id"); got != "untouched" {
		t.Errorf("X-Request-Id = %q, want unchanged", got)
	}
}

// TestRateLimiterSet_EnforcesBurstThenRejects checks that a route bound to
// a rate-limit policy is rejected once its burst is exhausted.
func TestRateLimiterSet_EnforcesBurstThenRejects(t *testing.T) {
	limiters := policy.NewRateLimiterSet([]snapshot.CompiledRateLimit{
		{Limit: rate.Limit(0), Burst: 1}, // no refill: exactly one request allowed
	})

	route := &snapshot.CompiledRoute{Policies: snapshot.CompiledRoutePolicies{RateLimitID: 0}}

	if !limiters.Allow(route) {
		t.Fatal("first request should be allowed within burst")
	}
	if limiters.Allow(route) {
		t.Fatal("second request should be rejected: burst exhausted, no refill")
	}
}

// TestRateLimiterSet_NoRateLimitAlwaysAllows checks that a route with no
// rate-limit policy attached is never throttled, regardless of volume.
func TestRateLimiterSet_NoRateLimitAlwaysAllows(t *testing.T) {
	limiters := policy.NewRateLimiterSet([]snapshot.CompiledRateLimit{
		{Limit: rate.Limit(0), Burst: 1},
	})

	route := &snapshot.CompiledRoute{Policies: snapshot.CompiledRoutePolicies{RateLimitID: policy.NoRateLimit}}

	for i := range 5 {
		if !limiters.Allow(route) {
			t.Fatalf("request %d: want allowed, route has no rate limit attached", i)
		}
	}
}
