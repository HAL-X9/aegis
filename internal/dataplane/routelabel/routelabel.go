// Package routelabel carries the matched route's stable identifier from the
// data-plane executor back up to the metrics middleware, without giving the
// executor a compile-time dependency on the metrics or middleware packages.
//
// The identifier is the route's configured name, never the raw request path:
// labelling by path would give every distinct ":id" its own Prometheus
// series, so label cardinality would be unbounded. Requests that match no
// route are reported as "unmatched", which keeps the label set bounded.
//
// Status: only the executor half is wired. The executor calls
// SetRoutePattern, but nothing creates a Recorder or attaches it with
// NewContext, and the metrics middleware does not read one, so the value is
// currently discarded (see "Roadmap / known limitations" in ARCHITECTURE.md).
// The intended design is for the metrics middleware to take a pooled Recorder,
// attach it to the request context, and read RoutePattern after the handler
// returns.
package routelabel

import "context"

type ctxKey struct{}

// Recorder holds the route identifier for a single in-flight request.
//
// Recorders are pooled by the metrics middleware and reset between
// requests via Reset — callers must not retain a Recorder, or the context
// it was placed into, past the request it was created for.
type Recorder struct {
	pattern string
}

// Reset clears any previously recorded pattern so a pooled Recorder can be
// reused safely for the next request.
func (r *Recorder) Reset() {
	r.pattern = ""
}

// SetRoutePattern records the identifier of the route the executor matched
// for the current request. Call at most once per request, right after a
// route match is found.
//
// Safe to call on a nil Recorder (no-op) so callers that run the executor
// without the metrics middleware in front of it — direct unit tests, for
// instance — don't need a nil check.
func (r *Recorder) SetRoutePattern(pattern string) {
	if r == nil {
		return
	}
	r.pattern = pattern
}

// RoutePattern returns the recorded pattern, or "unmatched" if none was set
// — e.g. requests that failed lookup, method matching, or engine/transport
// availability checks before a route was resolved. The fallback keeps the
// Prometheus "route" label always present and bounded.
func (r *Recorder) RoutePattern() string {
	if r == nil || r.pattern == "" {
		return "unmatched"
	}
	return r.pattern
}

// NewContext returns a copy of ctx carrying rec, for the metrics middleware
// to attach to the request before invoking the rest of the chain.
func NewContext(ctx context.Context, rec *Recorder) context.Context {
	return context.WithValue(ctx, ctxKey{}, rec)
}

// FromContext extracts the Recorder placed by NewContext, or nil if none is
// present.
func FromContext(ctx context.Context) *Recorder {
	rec, _ := ctx.Value(ctxKey{}).(*Recorder)
	return rec
}
