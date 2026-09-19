package proxy

import (
	"github.com/HAL-X9/aegis/internal/dataplane/policy"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
)

// View is the complete, request-ready data-plane state: everything
// Executor.ServeHTTP needs to serve one request. It is derived from a
// single snapshot.CompiledConfig and published as one atomic unit (see
// Executor.Publish), so a request in flight never sees a routing table
// from one config paired with rate limiters from another.
//
// Deliberately NOT part of View:
//   - http.RoundTripper — its connection pool outlives any single
//     configuration and must never be swapped on reload;
//   - snapshot.CompiledConfig itself — View holds the derived,
//     request-ready indices built from it (router.Engine,
//     policy.RateLimiterSet), not the source config.
//
// There is no reload path yet, but this boundary exists so that adding
// one later is a matter of building a new View and calling Publish, not
// a redesign of Executor.
type View struct {
	Engine   *router.Engine
	Limiters *policy.RateLimiterSet
}
