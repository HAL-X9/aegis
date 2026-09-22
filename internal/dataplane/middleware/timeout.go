package middleware

import (
	"context"
	"net/http"
	"time"
)

// TimeoutMiddleware applies a deadline to the entire downstream request
// handling chain.
type TimeoutMiddleware struct {
	timeout time.Duration
}

func NewTimeoutMiddleware(timeout time.Duration) *TimeoutMiddleware {
	return &TimeoutMiddleware{
		timeout: timeout,
	}
}

func (m *TimeoutMiddleware) Timeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.timeout <= 0 {
			next.ServeHTTP(w, r)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), m.timeout)
		defer cancel()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
