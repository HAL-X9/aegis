package policy

import (
	"net/netip"
	"testing"
	"time"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

var (
	t0      = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	limited = &snapshot.CompiledRoute{Policies: snapshot.CompiledRoutePolicies{RateLimitID: 0}}
)

func TestRateLimiterSetKeysBucketsByClient(t *testing.T) {
	tests := map[string]struct {
		first, second string
		shared        bool
	}{
		"distinct IPv4 addresses":       {first: "192.0.2.1", second: "192.0.2.2", shared: false},
		"one IPv6 /64":                  {first: "2001:db8:0:1::1", second: "2001:db8:0:1::2", shared: true},
		"distinct IPv6 /64s":            {first: "2001:db8:0:1::1", second: "2001:db8:0:2::1", shared: false},
		"IPv4-mapped IPv6 and its IPv4": {first: "::ffff:192.0.2.1", second: "192.0.2.1", shared: true},
		"IPv6 zones":                    {first: "fe80::1%eth0", second: "fe80::2%eth1", shared: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			limiters := NewRateLimiterSet([]snapshot.CompiledRateLimit{{Limit: 1, Burst: 1}})

			if !limiters.allowAt(limited, netip.MustParseAddr(tc.first), t0) {
				t.Fatal("first client rejected with a full bucket")
			}
			if got := !limiters.allowAt(limited, netip.MustParseAddr(tc.second), t0); got != tc.shared {
				t.Fatalf("second client rejected = %v, want %v", got, tc.shared)
			}
		})
	}
}

func TestRateLimiterSetSharesClientBucketAcrossRoutes(t *testing.T) {
	limiters := NewRateLimiterSet([]snapshot.CompiledRateLimit{{Limit: 1, Burst: 1}})
	other := &snapshot.CompiledRoute{Policies: snapshot.CompiledRoutePolicies{RateLimitID: 0}}
	client := netip.MustParseAddr("192.0.2.1")

	if !limiters.allowAt(limited, client, t0) {
		t.Fatal("first request rejected with a full bucket")
	}
	if limiters.allowAt(other, client, t0) {
		t.Fatal("second route admitted a client whose bucket for the same policy is empty")
	}
}

// TestRateLimiterSetSweepForgetsOnlyRefilledBuckets checks that a sweep never
// changes a decision: it forgets a bucket only once it is full again.
func TestRateLimiterSetSweepForgetsOnlyRefilledBuckets(t *testing.T) {
	limiters := NewRateLimiterSet([]snapshot.CompiledRateLimit{{Limit: 1, Burst: 2}})
	exhausted := netip.MustParseAddr("192.0.2.1")
	refilled := netip.MustParseAddr("192.0.2.2")

	limiters.allowAt(limited, exhausted, t0)
	limiters.allowAt(limited, exhausted, t0)
	limiters.allowAt(limited, refilled, t0)

	buckets := limiters.policies[0]
	buckets.sweep(t0.Add(time.Second))

	if _, ok := buckets.buckets[refilled]; ok {
		t.Error("a refilled bucket survived the sweep")
	}
	if _, ok := buckets.buckets[exhausted]; !ok {
		t.Fatal("a bucket that has not refilled was forgotten")
	}
	if !limiters.allowAt(limited, exhausted, t0.Add(time.Second)) {
		t.Error("the one token refilled after a second was lost")
	}
	if limiters.allowAt(limited, exhausted, t0.Add(time.Second)) {
		t.Error("the sweep granted a token the client had not earned")
	}
}

// TestRateLimiterSetBoundsTrackedClients sends one request from each of many
// clients, each arriving after the previous bucket has refilled, so memory
// must stay bounded however many distinct clients there are.
func TestRateLimiterSetBoundsTrackedClients(t *testing.T) {
	limiters := NewRateLimiterSet([]snapshot.CompiledRateLimit{{Limit: 1000, Burst: 1}})
	buckets := limiters.policies[0]

	now := t0
	for i := range 10 * minSweep {
		client := netip.AddrFrom4([4]byte{10, byte(i >> 16), byte(i >> 8), byte(i)})
		limiters.allowAt(limited, client, now)
		now = now.Add(time.Millisecond)

		if n := len(buckets.buckets); n > minSweep {
			t.Fatalf("after %d clients %d buckets are tracked, want at most %d", i+1, n, minSweep)
		}
	}
}

// TestRateLimiterSetDoesNotAllocateForKnownClient keeps the request path free
// of allocations once a client has a bucket.
func TestRateLimiterSetDoesNotAllocateForKnownClient(t *testing.T) {
	limiters := NewRateLimiterSet([]snapshot.CompiledRateLimit{{Limit: 1e9, Burst: 1}})

	for _, raw := range []string{"192.0.2.1", "2001:db8::1"} {
		client := netip.MustParseAddr(raw)
		limiters.Allow(limited, client)

		if allocs := testing.AllocsPerRun(100, func() { limiters.Allow(limited, client) }); allocs != 0 {
			t.Errorf("%s: Allow allocates %v times per call, want 0", raw, allocs)
		}
	}
}

func TestRateLimiterSetWithoutPolicyAllows(t *testing.T) {
	limiters := NewRateLimiterSet([]snapshot.CompiledRateLimit{{Limit: 1, Burst: 1}})
	client := netip.MustParseAddr("192.0.2.1")

	for name, route := range map[string]*snapshot.CompiledRoute{
		"nil route":      nil,
		"no rate limit":  {Policies: snapshot.CompiledRoutePolicies{RateLimitID: snapshot.NoRateLimit}},
		"unknown policy": {Policies: snapshot.CompiledRoutePolicies{RateLimitID: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			for range 3 {
				if !limiters.allowAt(route, client, t0) {
					t.Fatal("rejected without a rate-limit policy")
				}
			}
		})
	}
}
