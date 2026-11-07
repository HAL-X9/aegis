package policy

import (
	"net/netip"
	"sync"
	"time"

	"github.com/HAL-X9/aegis/internal/snapshot"
	"golang.org/x/time/rate"
)

// minSweep is the number of clients a policy tracks before it first sweeps.
const minSweep = 1024

// RateLimiterSet enforces the compiled rate-limit policies, indexed by the
// policy's identifier. Each policy gives every client a token bucket of its
// own, so a client that exhausts its bucket does not throttle any other. A
// client's bucket is shared by every route that references the policy.
// RateLimiterSet is safe for concurrent use.
type RateLimiterSet struct {
	policies []*clientBuckets
}

// NewRateLimiterSet creates the policies in order, so that a policy's index
// in compiled is its identifier.
func NewRateLimiterSet(compiled []snapshot.CompiledRateLimit) *RateLimiterSet {
	policies := make([]*clientBuckets, len(compiled))
	for i, c := range compiled {
		policies[i] = &clientBuckets{
			limit:   rate.Limit(c.Limit),
			burst:   c.Burst,
			buckets: make(map[netip.Addr]*rate.Limiter),
			sweepAt: minSweep,
		}
	}
	return &RateLimiterSet{policies: policies}
}

// Allow reports whether client may send a request under the route's
// rate-limit policy, taking a token from the client's bucket if so.
//
// client is the address of the connection's peer. An IPv6 client is
// identified by its /64 prefix (see clientKey). An invalid address, such as
// the zero Addr, identifies one client shared by every such request.
//
// A nil route, or any ID without a corresponding policy (including
// snapshot.NoRateLimit and an out-of-range ID from an unset field), is
// treated as "no limit" rather than panicking or silently picking policy 0.
func (s *RateLimiterSet) Allow(route *snapshot.CompiledRoute, client netip.Addr) bool {
	return s.allowAt(route, client, time.Now())
}

func (s *RateLimiterSet) allowAt(route *snapshot.CompiledRoute, client netip.Addr, now time.Time) bool {
	if route == nil {
		return true
	}

	id := route.Policies.RateLimitID
	if id < 0 || int(id) >= len(s.policies) {
		return true
	}
	return s.policies[id].allow(clientKey(client), now)
}

// clientKey returns the key of client's bucket. An IPv6 client is keyed by
// its /64 prefix: a subscriber is commonly assigned a whole /64 and would
// otherwise draw a fresh bucket from each of its addresses. An IPv4-mapped
// IPv6 address is keyed as the IPv4 address it carries.
func clientKey(client netip.Addr) netip.Addr {
	client = client.Unmap()
	if client.Is6() {
		prefix, _ := client.Prefix(64) // cannot fail: an IPv6 address has 128 bits
		return prefix.Addr()
	}
	return client
}

// clientBuckets holds the buckets of one policy, one per client key.
type clientBuckets struct {
	limit rate.Limit
	burst int

	mu      sync.Mutex
	buckets map[netip.Addr]*rate.Limiter
	// sweepAt is the number of buckets at which a new client triggers a sweep.
	sweepAt int
}

func (c *clientBuckets) allow(key netip.Addr, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	bucket, ok := c.buckets[key]
	if !ok {
		if len(c.buckets) >= c.sweepAt {
			c.sweep(now)
		}
		bucket = rate.NewLimiter(c.limit, c.burst)
		c.buckets[key] = bucket
	}
	return bucket.AllowN(now, 1)
}

// sweep forgets every client whose bucket is full again. A full bucket admits
// exactly what a new one would, so forgetting it changes no decision. A sweep
// runs once the number of buckets has doubled since the previous one, which
// keeps its cost amortized constant per new client and memory within twice
// the number of recent clients: a client is recent until its bucket refills,
// at most burst/rate after its last request.
func (c *clientBuckets) sweep(now time.Time) {
	for key, bucket := range c.buckets {
		if bucket.TokensAt(now) >= float64(c.burst) {
			delete(c.buckets, key)
		}
	}
	c.sweepAt = max(minSweep, 2*len(c.buckets))
}
