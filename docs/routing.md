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
2. `internal/controlplane/normalize` canonicalizes the validated model
   (path prefixes, method casing, header names, policy references).
3. `internal/controlplane/compile` transforms the normalized model into
   `snapshot.CompiledConfig`: a method bitmask per route
   (`contracts/methodmask`), precompiled header predicates
   (`snapshot.HeaderPredicate`), and a compiled per-service upstream origin
   string (`scheme://host:port`).
4. `router.BuildEngine` consumes the compiled snapshot. It builds a
   build-time radix tree (`BuildRadixTrie` / `RadixTrie.Insert`) keyed by
   route index, then flattens it once into an immutable, pointer-free
   `FlatTrie` arena (`Flatten`, in `flat.go`) — contiguous node, prefix, and
   route-reference slices addressed by `uint32` index rather than pointers.
   The build-time tree is discarded after flattening; only the flattened
   arena is retained.
5. The public HTTP server handles all paths through
   `internal/dataplane/proxy.Executor`, which calls `engine.Lookup`,
   applies method and header predicate filtering, and proxies the request
   to the selected upstream.

There is no hot config reload in the current implementation: the engine is
built once during `app.Bootstrap`. Changing routes requires a process
restart.

## Roles by package (as implemented)

`internal/controlplane/loader` loads the routes manifest and rejects
invalid documents before compilation.

`internal/controlplane/validate` enforces semantic constraints on the
unmarshaled model (non-empty name, `path_prefix` starting with `/`,
allowed methods, upstream scheme/host/port, and related rules).

`internal/controlplane/normalize` canonicalizes the validated model into a
deterministic intermediate representation (`internal/controlplane/ir`).

`internal/controlplane/compile` turns the normalized IR into
`snapshot.CompiledConfig`: `CompiledRoute` values (path prefix, method
bitmask, compiled header predicates, compiled policy plans), compiled
services, and compiled rate-limit definitions.

`internal/dataplane/router/index_insert.go` defines the build-time radix
tree (`RadixTrie`, `RadixNode`) and its insertion logic: `Insert` walks a
route's `path_prefix` string segment by segment (`:` parameter edges, `*`
wildcard edges, static edges with radix compression), attaching a route
index (not a pointer) at each terminal node. This structure exists only
during engine construction; the request path never touches it.

`internal/dataplane/router/flat.go` defines `FlatTrie`, the immutable
structure actually queried at request time. `Flatten` performs a
breadth-first traversal of the build-time `RadixTrie`, producing three
contiguous slices — `nodes []FlatNode`, `prefixes []byte`,
`routeRefs []uint32` — with all cross-references expressed as slice
indices. `FlatTrie.Lookup` walks this arena without allocating: candidate
route indices are returned as a sub-slice of `routeRefs`, never copied.

`internal/dataplane/router/engine.go` is the façade used by the proxy: it
holds the flattened trie, the compiled route slice, and one parsed
`*url.URL` per service (indexed by `snapshot.ServiceID`). It exposes
`Lookup(path string) []uint32`, `Route(id uint32) *snapshot.CompiledRoute`,
and `UpstreamURL(route *snapshot.CompiledRoute) *url.URL`. Callers outside
this package must not depend on `FlatTrie`, `FlatNode`, or `RadixTrie`
internals.

`internal/dataplane/proxy/executor.go` orchestrates matching for each
request: it calls `Lookup` on the path, filters candidates by permitted
HTTP method, applies header predicate matching via `router.HeadersMatch`,
constructs the outbound request from the compiled upstream origin plus the
incoming path and query, and performs the upstream round trip.

## Data boundaries

`snapshot.CompiledRoute` (from `compile`) is the immutable route
definition held in `Engine.routes`, addressed by index.

Runtime output of path resolution is `[]uint32` — route indices into
`Engine.routes` — not a wrapper type and not a slice of pointers. Method
and header disambiguation are handled by the executor after lookup, by
resolving each candidate index via `Engine.Route`.

## Path matching behavior (implemented)

Trie construction uses the literal `match.path_prefix` string from
configuration (validated to start with `/`). Segments separated by `/` are
processed in order:

Static segments match exact byte equality, using radix compression: a
shared prefix across multiple routes is stored once and split only where
routes diverge. A segment whose first byte is `:` is evaluated through the
single `ParamChild` edge at that depth; parameter names are not
distinguished during lookup. A segment whose first byte is `*` is
evaluated through `WildcardChild`; lookup may return wildcard candidates
without consuming the remainder of the path.

Lookup does not allocate: `FlatTrie.Lookup` walks indices into
pre-allocated slices and returns a window into `routeRefs`. It returns
`nil` when no branch matches.

**Overlap and ordering:** multiple routes can register candidates on the
same trie node. The executor iterates candidates in the order recorded at
build time (insertion order for routes sharing a terminal node) and picks
the first candidate that satisfies both method and header predicates.
There is no separate global route-priority field; stable behavior for
overlapping prefixes depends on compile-time route order.

## Method matching

At configuration time, `methodmask.BuildMethodMask` compiles the declared
method list into a bitmask. An empty method list is interpreted as
`MethodAll`. At runtime, `methodmask.MethodBit` classifies the incoming
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
are rejected during compilation.

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
wiring. `FlatTrie`'s underlying slices are never mutated after
construction, so concurrent `Lookup` calls require no synchronization. A
future reload feature would build a new `Engine` off the request path and
publish it via an atomic pointer swap; that pattern is not implemented
today.

## Design guardrails (unchanged intent)

- **Compile / index / runtime separation:** the proxy must keep using
  `router.Engine`, not `FlatTrie` or `RadixTrie` internals.
- **Invalid config fails before traffic:** validation and compilation
  errors surface during load/bootstrap, not on individual requests.
- **Lookup must not panic** on malformed request paths: `FlatTrie.Lookup`
  walks bytes defensively and returns no match when branches fail.

Future extensions (query predicates, route scoring, config reload,
expanded middleware, metrics) should preserve the compile/build/runtime
boundary so the index can evolve without rewriting the executor's role.