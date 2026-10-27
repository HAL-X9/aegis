// Package proxy implements the reverse-proxy executor of the dataplane.
//
// Executor resolves a request through the routing engine, enforces the
// route's rate limit, applies header mutations, forwards the request to the
// selected upstream, and streams the response back to the client. The active
// View (routing engine and rate limiters) is published atomically, so the
// request path reads it without locking.

package proxy
