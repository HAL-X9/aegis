# Routing Engine Architecture

## Intent

This document specifies routing behavior in Aegis as implemented: how
configuration is compiled, how runtime lookup candidates are produced, and
how the final route decision is made in the data plane. It also defines
package boundaries that must not be violated.

## Current end-to-end flow

At process startup:

1. Routes are loaded from YAML via `internal/controlplane/loader`, which
   unmarshals with strict field checking and runs
   `internal/controlplane/validate`.
2. `internal/controlplane/pipeline.Build` passes the validated model through
   `internal/controlplane/normalize`, which canonicalizes it (path prefixes,
   method casing, header names, policy references).
3. `internal/controlplane/compile` transforms the normalized model into
   `snapshot.CompiledConfig`: a method bitmask per route
   (`internal/httpmethod`), precompiled header predicates
   (`snapshot.HeaderPredicate`), and a parsed per-service upstream origin
   (`*url.URL`, scheme and host only).
4. `compile.Index` builds the path index into `CompiledConfig.Index`. It
   inserts every route into a build-time radix tree keyed by route index
   (`radixTrie.insert`, in `index_insert.go`), then flattens it once into an
   immutable, pointer-free `snapshot.RouteIndex` arena (`flatten`, in
   `index_flatten.go`) — contiguous node, prefix, and route-reference slices
   addressed by index rather than pointers. The build-time tree is discarded
   after flattening; only the arena is retained.
5. `router.NewEngine` wraps the compiled snapshot for request-time use. It
   performs no computation of its own.
6. `app.Bootstrap` publishes the engine, together with the rate limiters, as
   one `proxy.View` (`Executor.Publish`). The public HTTP server hands all
   paths to `internal/dataplane/proxy.Executor`, which canonicalizes the path,
   calls `engine.Resolve` to select the route, and proxies the request to
   the selected upstream.

There is no hot config reload in the current implementation: the `View` is
published once during `app.Bootstrap`. Changing routes requires a process
restart.

## Roles by package (as implemented)

`internal/controlplane/loader` loads the routes manifest and rejects
invalid documents before compilation.

`internal/controlplane/validate` enforces every rule that depends on the
configuration alone: the constraints of each section (non-empty name,
`path_prefix` starting with `/`, allowed methods, upstream scheme/host/port,
retry bounds), references between sections (routes to services and
policies), and header conflicts between the policies a route combines. Its
`Validated` output is complete input for compilation.

`internal/controlplane/normalize` canonicalizes the validated model into a
deterministic intermediate representation (`internal/controlplane/ir`).

`internal/controlplane/compile` turns the normalized IR into
`snapshot.CompiledConfig`: `CompiledRoute` values (path prefix, method
bitmask, compiled header predicates, compiled policy plans), compiled
services, and compiled rate-limit definitions.

`internal/controlplane/compile/index_insert.go` defines the build-time radix
tree (`radixTrie`, `radixNode`) and its insertion logic: `insert` walks a
route's `path_prefix` segment by segment (`:` parameter edges, `*`
wildcard edges, static edges with radix compression), attaching a route
index (not a pointer) at each terminal node. This structure exists only
while the control plane builds the index; the request path never touches it.

`internal/controlplane/compile/index_flatten.go` defines `Index`, which
builds that tree from the compiled routes, and `flatten`, which converts it
into `snapshot.RouteIndex`: a breadth-first traversal producing three
contiguous arrays — a node array, a single prefix string, and a route-reference
array — with all cross-references expressed as indices. The fields of
`RouteIndex` are unexported and the constructor copies its inputs, so nothing
outside `internal/snapshot` can modify an index once it is built.

`internal/snapshot/index.go` defines `RouteIndex` and its `Lookup`, the
immutable structure actually queried at request time. `Lookup` is a visitor
interface: it calls `visit` with each structurally matching candidate set, in
priority order, and stops as soon as `visit` returns `true`. It walks the
arena without allocating: each candidate set is a sub-slice of the
route-reference array, never copied.

`internal/dataplane/router/engine.go` is the façade used by the proxy:
`NewEngine` wraps the compiled snapshot — the route index, the compiled
route slice, and the services (each carrying its parsed upstream URL). It
exposes `Lookup(path string, visit func([]snapshot.RouteID) bool)`,
`Route(id snapshot.RouteID) *snapshot.CompiledRoute`,
and `UpstreamURL(route *snapshot.CompiledRoute) *url.URL`. Callers outside
this package must not depend on the index internals.

`router.Engine.Resolve` selects the route for a request: it calls `Lookup` on
the path, filters candidates by permitted HTTP method, applies header
predicate matching via `router.HeadersMatch`, and reports `Matched`,
`NotFound`, or `MethodNotAllowed`. `internal/dataplane/proxy/executor.go`
translates a failed outcome into `404` or `405` and otherwise constructs the
outbound request from the compiled upstream origin plus the incoming path and
query, and performs the upstream round trip.

## Data boundaries

`snapshot.CompiledRoute` (from `compile`) is the immutable route
definition held in `Engine.routes`, addressed by index.

Path resolution hands the visitor a `[]snapshot.RouteID` (a `uint32`
type) — route indices into `Engine.routes` — not a wrapper type and not a
slice of pointers. Method and header disambiguation are handled by the
executor inside the visitor, by resolving each candidate index via
`Engine.Route`. Because the visitor can reject a candidate set, `Lookup`
continues with lower-priority sets (see `routing-and-errors.md` §1.4).

## Path matching behavior (implemented)

Trie construction uses the literal `match.path_prefix` string from
configuration (validated to start with `/`). Segments separated by `/` are
processed in order:

Static segments match by exact byte equality, using radix compression: a
shared prefix across multiple routes is stored once and split only where
routes diverge. Each static segment is stored together with its
terminating `/`, so a node whose prefix ends in `/` marks a segment
boundary and `/abc` and `/ab/c` never share one. Only boundary nodes carry
routes and `:`/`*` edges. A segment whose first byte is `:` is evaluated
through the single `ParamChild` edge at that depth; parameter names are not
distinguished during lookup. A segment whose first byte is `*` is
evaluated through `WildcardChild`; lookup may return wildcard candidates
without consuming the remainder of the path.

`Lookup` expects a canonical request path (`request.CanonicalPath`, see
`docs/routing-and-errors.md` §1.0). It never collapses `//` itself: an
empty segment simply fails to match, so the router cannot route a path
other than the one forwarded upstream.

Lookup does not allocate: `RouteIndex.Lookup` walks indices into
pre-allocated slices and passes windows into the route-reference array to
the visitor. If no branch matches, the visitor is never called.

**Overlap and ordering:** multiple routes can register candidates on the
same trie node. The executor iterates candidates in the order recorded at
build time (the order of routes in the manifest, for routes sharing a
terminal node) and picks the first candidate that satisfies both method and
header predicates.
There is no separate global route-priority field; stable behavior for
overlapping prefixes depends on compile-time route order.

## Method matching

At configuration time, `httpmethod.MaskOf` compiles the declared
method list into a bitmask. An empty method list is interpreted as
`httpmethod.All`. At runtime, `httpmethod.Bit` classifies the incoming
method; unsupported methods are rejected with `405` before candidate
selection proceeds.

## Header matching

At configuration time, `compile.headersPredicate` (in
`routes_compiler.go`) compiles `match.headers` into deterministic
`[]snapshot.HeaderPredicate`. Header keys are sorted to provide stable
evaluation order. An empty value list (`[]`) is treated as a
presence-only constraint. A non-empty value list is treated as an
exact-value allowlist. Duplicate allowed values are removed while
preserving first-seen order. Empty header names and empty allowed values
are rejected during validation.

At runtime, `router.HeadersMatch` evaluates all compiled predicates using
logical AND semantics. For allowlist predicates, matching succeeds when at
least one request value equals at least one allowed value. If a route has
no header predicates, header matching is true by definition.

After path lookup, `Executor` applies response semantics as follows. If no
candidate supports the request method, the response is
`405 Method Not Allowed`. If at least one candidate supports the method
but no candidate satisfies header predicates, the response is
`404 Not Found`. Otherwise, the first candidate that satisfies both method
and header predicates is selected and proxied.

## Mutability and concurrency

The `Engine` is created once for the process lifetime in the current app
wiring. `RouteIndex`'s underlying slices are never mutated after
construction, so concurrent `Lookup` calls require no synchronization. The
`Executor` already holds its `proxy.View` behind an `atomic.Pointer`, so a
future reload feature would build a new `Engine` and rate limiters off the
request path and install them with a second `Executor.Publish`. Only the
initial publication exists today.

## Design guardrails (unchanged intent)

- **Compile / index / runtime separation:** the proxy must keep using
  `router.Engine`, not `RouteIndex` or radix internals.
- **Invalid config fails before traffic:** validation and compilation
  errors surface during load/bootstrap, not on individual requests.
- **Lookup must not panic** on malformed request paths: `RouteIndex.Lookup`
  walks bytes defensively and returns no match when branches fail.

Future extensions (query predicates, route scoring, config reload,
expanded middleware, metrics) should preserve the compile/build/runtime
boundary so the index can evolve without rewriting the executor's role.