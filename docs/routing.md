# Routing Engine Architecture

## Intent

This document specifies routing behavior in Aegis **as implemented**: how configuration is compiled, how runtime lookup candidates are produced, and how the final route decision is made in the dataplane. It also defines package boundaries that must not be violated.

## Current end-to-end flow

At process startup:

1. Routes are loaded from YAML via `internal/controlplane/loader`, which unmarshals with strict field checking and runs `internal/controlplane/validate`.
2. `router.BuildEngine` calls `compiler.Compile`, wraps each compiled route in a `RouteIndexEntry`, and builds a radix trie via `BuildRadixTrie` / `Insert`.
3. The public HTTP server handles all paths through `internal/dataplane/proxy.Executor`, which calls `engine.Lookup`, applies method and header predicate filtering, and proxies the request to the selected upstream.

There is **no** hot config reload in the current implementation: the engine is built once during `app.Bootstrap`. Changing routes requires restart (or future reload support not described here as present).

## Roles by package (as implemented)

`internal/controlplane/loader` loads the routes manifest and rejects invalid documents before compilation.

`internal/controlplane/validate` enforces semantic constraints on the unmarshaled model (non-empty name, `path_prefix` starting with `/`, allowed methods, upstream scheme/host/port, and related rules).

`internal/controlplane/compiler` turns the validated model into `CompiledRoute` values: path prefix, method bitmask, precompiled header predicates (`CompiledMatch.Headers`), and a precomputed upstream origin string (`scheme://host:port`).

`internal/dataplane/router/index_build.go`, `index_insert.go`, and `index_lookup.go` own the radix index: insertion walks the route `path_prefix` string segment by segment (`:` parameter edges, `*` wildcard edges, static edges), and lookup returns candidate `RouteIndexEntry` values for a request path.

`internal/dataplane/router/engine.go` is the façade used by the proxy: it holds the trie and exposes `Lookup([]byte)`. Callers outside this package should not depend on trie node types.

`internal/dataplane/proxy/executor.go` orchestrates matching for each request: it calls `Lookup` on the path, filters candidates by permitted HTTP method, applies header predicate matching via `router.HeadersMatch`, constructs the outbound URL from the compiled upstream origin plus `r.URL.EscapedPath()`, and performs the upstream round trip. Middleware packages under `internal/dataplane/middleware` exist but are **not** composed into this path yet.

## Data boundaries

**`CompiledRoute`** (from `compiler`) is the immutable route definition consumed by the index entries.

**`RouteIndexEntry`** wraps `*CompiledRoute` for trie terminal nodes; multiple routes may share a terminal node (candidates slice).

Runtime output of path resolution is **`[]*RouteIndexEntry`**, not a separate `MatchResult` type. Method and header disambiguation semantics are handled in the executor after lookup.

## Path matching behavior (implemented)

Trie construction uses the literal `match.path_prefix` string from configuration (validated to start with `/`). Segments separated by `/` are processed in order:

Static segments match exact byte equality. A segment whose first byte is `:` is evaluated through the single `paramChild` edge at that depth; parameter names are not distinguished during lookup. A segment whose first byte is `*` is evaluated through `wildcardChild`; lookup may return wildcard candidates without consuming the remainder of the path, as implemented in `index_lookup.go`.

Lookup **does not** allocate per character beyond existing structures; it returns `nil` when no branch matches.

**Overlap and ordering:** multiple routes can register candidates on the same trie node. The executor iterates candidates in trie storage order and picks the **first** candidate that satisfies both method and header predicates. There is no separate global “route priority” field yet; stable behavior for overlapping prefixes depends on compile-time route order and trie insertion order.

## Method matching

At configuration time, `methodmask.BuildMethodMask` compiles the declared method list into a bitmask. An empty method list is interpreted as `MethodAll`. At runtime, `methodmask.MethodBit` classifies the incoming method; unsupported methods are rejected with `405` before candidate selection proceeds.

## Header matching

At configuration time, `compiler.BuildHeadersPredicate` compiles `match.headers` into deterministic `[]HeaderPredicate`. Header keys are sorted to provide stable evaluation order. An empty value list (`[]`) is treated as a presence-only constraint. A non-empty value list is treated as an exact-value allowlist. Duplicate allowed values are removed while preserving first-seen order. Empty header names and empty allowed values are rejected during compilation.

At runtime, `router.HeadersMatch` evaluates all compiled predicates using logical AND semantics. For allowlist predicates, matching succeeds when at least one request value equals at least one allowed value. If a route has no header predicates, header matching is true by definition.

After path lookup, `Executor` applies response semantics as follows. If no candidate supports the request method, the response is `405 Method Not Allowed`. If at least one candidate supports the method but no candidate satisfies header predicates, the response is `404 Not Found`. Otherwise, the first candidate that satisfies both method and header predicates is selected and proxied.

## Mutability and concurrency

The engine struct is **created once** for the process lifetime in the current app wiring. The trie and candidate slices are not guarded for concurrent mutation because nothing mutates them after build. A future reload feature would need a **new** engine built off the request path and an atomic pointer swap; that pattern is **not** implemented today.

## Operational / observability

The architecture doc previously listed build and lookup metrics; **they are not implemented** in the routing stack yet. Health endpoints exist on the system listener (see README), but routing-specific counters and histograms are future work.

## Design guardrails (unchanged intent)

- **Compile / index / runtime separation:** the proxy must keep using `router.Engine` (or a similarly narrow API), not trie internals.
- **Invalid config fails before traffic:** validation and compilation errors surface during load/bootstrap, not on individual requests.
- **Lookup must not panic** on malformed request paths: current lookup code walks bytes defensively and returns no match when branches fail.

Future extensions (header/query predicates, route scoring, config reload, middleware pipeline, metrics) should preserve the compile/build/runtime boundary so the index can evolve without rewriting the executor’s role.
