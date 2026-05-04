# Routing Engine Architecture

## Intent

This document describes how routing works in Aegis **today**: how configuration becomes a lookup structure, where matching happens, and which layers must not leak into each other. The design still aims at a clean split between configuration compilation, index construction, and request-time matching; sections below call out what is **implemented** versus what is **not yet wired** in the codebase.

## Current end-to-end flow

At process startup:

1. Routes are loaded from YAML via `internal/controlplane/loader`, which unmarshals with strict field checking and runs `internal/controlplane/validate`.
2. `router.BuildEngine` calls `compiler.Compile`, wraps each compiled route in a `RouteIndexEntry`, and builds a radix trie via `BuildRadixTrie` / `Insert`.
3. The public HTTP server handles all paths with a single dataplane entrypoint: `internal/dataplane/proxy.Executor`, which calls `engine.Lookup` and then applies HTTP method selection and proxying.

There is **no** hot config reload in the current implementation: the engine is built once during `app.Bootstrap`. Changing routes requires restart (or future reload support not described here as present).

## Roles by package (as implemented)

`internal/controlplane/loader` loads the routes manifest and rejects invalid documents before compilation.

`internal/controlplane/validate` enforces semantic constraints on the unmarshaled model (non-empty name, `path_prefix` starting with `/`, allowed methods, upstream scheme/host/port, and related rules).

`internal/controlplane/compiler` turns the validated model into `CompiledRoute` values: path prefix, method bitmask, and a precomputed upstream origin string (`scheme://host:port`). **Header match fields exist on the YAML model and on `CompiledMatch` in types, but compilation does not populate header predicates yet; the executor does not evaluate them.**

`internal/dataplane/router/index_build.go`, `index_insert.go`, and `index_lookup.go` own the radix index: insertion walks the route `path_prefix` string segment by segment (`:` parameter edges, `*` wildcard edges, static edges), and lookup returns candidate `RouteIndexEntry` values for a request path.

`internal/dataplane/router/engine.go` is the façade used by the proxy: it holds the trie and exposes `Lookup([]byte)`. Callers outside this package should not depend on trie node types.

`internal/dataplane/proxy/executor.go` orchestrates matching for each request: it calls `Lookup` on the path, filters candidates by permitted HTTP method, constructs the outbound URL from the compiled upstream origin plus `r.URL.EscapedPath()`, and performs the upstream round trip. Middleware packages under `internal/dataplane/middleware` exist but are **not** composed into this path yet.

## Data boundaries

**`CompiledRoute`** (from `compiler`) is the immutable route definition consumed by the index entries.

**`RouteIndexEntry`** wraps `*CompiledRoute` for trie terminal nodes; multiple routes may share a terminal node (candidates slice).

Runtime output of path resolution is **`[]*RouteIndexEntry`**, not a separate `MatchResult` type. Method disambiguation and “no matching method” semantics are handled in the executor after lookup.

## Path matching behavior (implemented)

Trie construction uses the literal `match.path_prefix` string from configuration (validated to start with `/`). Segments separated by `/` are processed in order:

- Static segments match exact byte equality on that segment.
- A segment whose first byte is `:` uses the single `paramChild` edge at that depth (named parameter segments are not distinguished by name during lookup—they share one edge).
- A segment whose first byte is `*` uses the `wildcardChild` edge; lookup may return wildcard candidates without consuming the rest of the path (prefix-style wildcard behavior per `index_lookup.go`).

Lookup **does not** allocate per character beyond existing structures; it returns `nil` when no branch matches.

**Overlap and ordering:** multiple routes can register candidates on the same trie node. The executor iterates candidates in trie storage order and picks the **first** whose method bitmask matches the request. There is no separate global “route priority” field yet; stable behavior for overlapping prefixes depends on compile-time route order and trie insertion order.

## Method matching

- Configuration-time: `methodmask.BuildMethodMask` builds a bitmask; an **empty** method list in YAML means “all methods allowed” (`MethodAll`).
- Runtime: `methodmask.MethodBit` classifies the request method; unknown methods yield `405` before upstream selection.

## Mutability and concurrency

The engine struct is **created once** for the process lifetime in the current app wiring. The trie and candidate slices are not guarded for concurrent mutation because nothing mutates them after build. A future reload feature would need a **new** engine built off the request path and an atomic pointer swap; that pattern is **not** implemented today.

## Operational / observability

The architecture doc previously listed build and lookup metrics; **they are not implemented** in the routing stack yet. Health endpoints exist on the system listener (see README), but routing-specific counters and histograms are future work.

## Design guardrails (unchanged intent)

- **Compile / index / runtime separation:** the proxy must keep using `router.Engine` (or a similarly narrow API), not trie internals.
- **Invalid config fails before traffic:** validation and compilation errors surface during load/bootstrap, not on individual requests.
- **Lookup must not panic** on malformed request paths: current lookup code walks bytes defensively and returns no match when branches fail.

Future extensions (header/query predicates, route scoring, config reload, middleware pipeline, metrics) should preserve the compile/build/runtime boundary so the index can evolve without rewriting the executor’s role.
