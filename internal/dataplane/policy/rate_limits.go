package policy

import (
	"github.com/HAL-X9/aegis/internal/snapshot"
	"golang.org/x/time/rate"
)

// RateLimiterSet holds one token-bucket limiter per compiled rate-limit
// policy, indexed by the policy's identifier. A limiter is shared by every
// route that references its policy and is safe for concurrent use.
type RateLimiterSet struct {
	limiters []*rate.Limiter
}

// NewRateLimiterSet creates a limiter for each compiled policy, in order, so
// that a policy's index in compiled is its identifier.
func NewRateLimiterSet(compiled []snapshot.CompiledRateLimit) *RateLimiterSet {
	limiters := make([]*rate.Limiter, len(compiled))
	for i, c := range compiled {
		limiters[i] = rate.NewLimiter(rate.Limit(c.Limit), c.Burst)
	}
	return &RateLimiterSet{limiters: limiters}
}

// Allow reports whether the request is permitted under the referenced
// rate-limit policy. A nil route, or any ID without a corresponding
// limiter — including snapshot.NoRateLimit and any out-of-range zero-value ID from
// an unset field — is treated as "no limit" rather than panicking or
// silently picking policy 0.
func (s *RateLimiterSet) Allow(route *snapshot.CompiledRoute) bool {
	if route == nil {
		return true
	}

	id := route.Policies.RateLimitID
	if id < 0 || int(id) >= len(s.limiters) {
		return true
	}
	return s.limiters[id].Allow()
}
