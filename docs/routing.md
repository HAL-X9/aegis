# Routing Engine Architecture

## Intent

This document describes the target routing architecture for Aegis and the contract between the router and the proxy runtime. The main design decision is strict phase separation: configuration is compiled once, an index is built once, and request matching stays minimal on the hot path. The purpose is not style purity; it is operational predictability under load.

## System Model

At startup (and on each accepted config reload), the control-plane manifest is transformed into an immutable runtime engine. The proxy does not parse routing config and does not interact with trie internals directly. It asks the engine to match a request path and then continues with method checks, middleware execution, and upstream handling.

The production flow is therefore:

1. `BuildEngine(config)` creates a new engine instance.
2. `Compile(config)` validates and normalizes routes.
3. Index build code converts compiled routes into index-ready entries.
4. Trie is constructed and attached to the engine.
5. Proxy uses only engine-level lookup/match API per request.

This boundary is mandatory. Any direct trie access from `proxy` is considered an architecture violation.

## Roles by Package

`router/compiler.go` owns semantic validation and normalization. It accepts control-plane input and returns canonical compiled routes. If a manifest is invalid, failure must happen here or immediately after, before the engine is exposed to traffic.

`router/index_build.go` and `router/index_insert.go` own index materialization. Their job is to take compiled routes and shape them for efficient lookup, including any precomputed metadata that can remove work from runtime.

`router/index_lookup.go` owns path matching against the trie. It should return deterministic candidates and avoid avoidable allocations.

`router/engine.go` is the only public runtime façade. It contains immutable routing state and exposes a stable API (`Lookup`/`Match`) consumed by the proxy.

`proxy/executor.go` orchestrates request execution. It must depend on `router.Engine` only, never on trie node types or insertion logic.

## Data Boundaries

`CompiledRoute` represents route semantics after compilation: this is the business-correct route definition used by the dataplane.

`RouteIndexEntry` (if retained) represents a lookup-optimized projection of `CompiledRoute`. It exists to cache precomputed fields needed for fast and deterministic matching. If no extra metadata is needed, the index may store compiled routes directly; if metadata is needed, index entries are the right extension point.

`MatchResult` (or equivalent) represents runtime output consumed by the executor. Keeping this separate from index internals prevents leakage of implementation details into the proxy layer.

## Determinism Rules

Path resolution must be deterministic and documented. For overlapping patterns, static segments are evaluated before parameter segments, and parameter segments before wildcards. When multiple routes remain equivalent after path evaluation, tie-breaking must follow a stable rule (for example explicit priority or original route order).

Method constraints can be applied as a post-path stage in the executor or as precomputed checks in the index layer, but behavior must remain equivalent and reproducible.

## Mutability and Reload Safety

Runtime engine state is immutable. Reload builds a new engine off-path; activation is an atomic swap. The active engine is never modified in place. This keeps the hot path lock-light and avoids partial-state visibility under concurrency.

If reload compilation or index build fails, the previous engine stays active. Failed builds never partially replace runtime state.

## Operational Requirements

Routing failures at build time must be explicit and actionable: invalid schema, semantic conflicts, unsupported patterns, or internal index inconsistencies. Runtime lookup must not panic on malformed request paths.

Observability must distinguish cold-path build health from hot-path matching behavior. At minimum, operators need build success/failure metrics, build latency, active route count, lookup latency, and match/no-match counters.

Performance expectations are straightforward: request-time matching should scale with path structure, not total route count, and heap activity on the hot path should be tightly controlled.

## Design Guardrails

The architecture may evolve (header/query predicates, route scoring, multi-tenant partitions), but one rule is fixed: compile/build/runtime separation must remain intact. As long as that boundary is preserved, index internals can change without forcing proxy rewrites.

