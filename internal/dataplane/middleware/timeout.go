package middleware

import (
	"context"
	"net/http"
	"time"
)

// TimeoutMiddleware applies a deadline to the request context of the handler
// chain it wraps. It never writes a response itself: the 504 for an expired
// deadline is produced by proxy.Executor when the upstream exchange fails
// with a timeout. Once the response headers are sent, an expiry can only
// truncate the body.
type TimeoutMiddleware struct {
	timeout time.Duration
}

// NewTimeoutMiddleware returns a middleware with the given deadline. A zero or
// negative timeout disables the deadline.
func NewTimeoutMiddleware(timeout time.Duration) *TimeoutMiddleware {
	return &TimeoutMiddleware{
		timeout: timeout,
	}
}

// Timeout wraps next so that every request context carries the deadline.
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
