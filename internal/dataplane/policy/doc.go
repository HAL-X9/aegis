// Package policy applies compiled route policies at request time.
//
// It provides token-bucket rate limiting, with one shared limiter per
// compiled policy, and the execution of compiled header mutation plans on
// requests and responses.

package policy
