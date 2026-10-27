// Package router resolves an incoming request against a compiled snapshot.
//
// Engine matches the request path through the precomputed route index,
// following the precedence static, parameter, wildcard, and then applies the
// method and header predicates. The result is reported as Matched, NotFound,
// or MethodNotAllowed. Resolution does not allocate.

package router
