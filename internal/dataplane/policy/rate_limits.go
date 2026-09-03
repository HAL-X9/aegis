package policy

import (
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
	"golang.org/x/time/rate"
)

// NoRateLimit marks a route as having no rate-limit policy attached.
const NoRateLimit = -1

type RateLimiterSet struct {
	limiters []*rate.Limiter
}

func NewRateLimiterSet(compiled []snapshot.CompiledRateLimit) *RateLimiterSet {
	limiters := make([]*rate.Limiter, len(compiled))
	for i, c := range compiled {
		limiters[i] = rate.NewLimiter(c.Limit, c.Burst)
	}
	return &RateLimiterSet{limiters: limiters}
}

// Allow reports whether the request is permitted under the referenced
// rate-limit policy. Any ID without a corresponding limiter — including
// NoRateLimit and any out-of-range zero-value ID from an unset field —
// is treated as "no limit" rather than panicking or silently picking
// policy 0.
func (s *RateLimiterSet) Allow(route *snapshot.CompiledRoute) bool {
	id := route.Policies.RateLimitID
	if id < 0 || int(id) >= len(s.limiters) {
		return true
	}
	return s.limiters[id].Allow()
}
