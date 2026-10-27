// Package admin implements the handlers of the system listener.
//
// It exposes the liveness and readiness probes and the Prometheus metrics
// endpoint, restricted to read-only (GET) access and kept separate from the
// public traffic path.

package admin
