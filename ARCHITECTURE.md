# Architecture

## Overview

Aegis is split into two planes with a hard boundary between them:

- **Control plane** — loads YAML configuration, normalizes it, validates it, and compiles it into an immutable snapshot. This runs at startup (and on reload), never on the request path.
- **Data plane** — serves live HTTP traffic against the compiled snapshot: route lookup, method/header matching, policy mutation, and proxying to upstream services.

The boundary exists so the hot path never re-derives behavior from raw configuration. Every request touches only precompiled, read-only structures — no YAML parsing, no validation, no config-shaped branching at request time.

```
configs/*.yaml
      │
      ▼
controlplane/loader → normalize → compile → validate → snapshot
                                                            │
                                                            ▼
                                                  dataplane/router.Engine
                                                            │
Client ──▶ edge/public ──▶ middleware chain ──▶ proxy.Executor
                              (recovery,             │
                               request id,           ▼
                               metrics,          Upstream service
                               timeout)
```

## Request lifecycle

1. `edge/public` accepts the connection and forwards to the data plane's compiled `http.Handler`.
2. The middleware chain runs in order: **recovery** (innermost safety net for panics) → **request ID** (generates a UUIDv7, attaches it to `context.Context`, echoes it to the response) → **metrics** → **timeout**.
3. `proxy.Executor` looks up the request path in a radix trie (`router.Engine.Lookup`), producing all route candidates that share that path.
4. Candidates are filtered by HTTP method using a precomputed bitmask (`contracts/methodmask`) and by header predicates (`router.HeadersMatch`). The first candidate satisfying both wins.
5. `policy.ExecuteMutations` applies configured header mutations to the outbound request, then hop-by-hop headers are stripped (`request.RemoveHopHeaders`).
6. The request is forwarded via the configured `http.RoundTripper`. On success, response headers are copied back, response-side policy mutations are applied, and the body is streamed to the client with `io.Copy`.
7. Failure modes are explicit and mapped to standard status codes: `503` (engine unavailable), `404` (no route), `405` (route exists, method doesn't), `502` (upstream failure).

## Key design decisions
````
**Radix trie routing, not a map.** Route lookup uses a compressed radix trie supporting static, `:param`, and `*wildcard` segments, matched with static > param > wildcard priority. Benchmarks show **0 allocations and sub-100ns lookups from 16 to 32,768 routes** — the tree stays shallow because of prefix compression, so lookup cost grows with path depth, not route count. Insertion (setup-time only) favors correctness over allocation avoidance; lookup (request-time) is optimized for zero allocations, since it runs on every request.

**Method matching via bitmask, not string comparison.** `contracts/methodmask` maps each HTTP method to a bit; route admission is a single `AND` against a precomputed mask instead of iterating and string-comparing allowed methods per request.

**Control/data plane separation.** Configuration compiles once into a `snapshot` package the data plane only reads. This means adding a route, a policy, or a service doesn't change data-plane code paths — it changes what the compiler produces, keeping the request-serving code free of configuration-shaped conditionals.

**Middleware as `func(http.Handler) http.Handler`.** Standard, composable Go idiom. A `chain.go` helper composes an ordered list of middleware into a single handler; `dataplane/pipeline` composes the resulting chain with routing, policy, and the executor into the full data-plane handler.

**Request ID: UUIDv7, generated server-side, never trusted from the client.** UUIDv7 is time-ordered, so IDs sort naturally in logs and any future datastore, unlike UUIDv4. Client-supplied `X-Request-ID` values are not used as the canonical ID — Aegis always generates its own, since an untrusted, unvalidated client header must never become an internal correlation key or a log-injection vector. The ID is carried through `context.Context` within the process and would be forwarded as a header/gRPC-metadata entry at any process boundary.

**Separate system and public listeners.** Health and metrics are served on a distinct port (`:18080`) from user traffic (`:8080`), so operational endpoints are never reachable through the same routing/policy path as untrusted traffic.

## Package layout

| Package                         | Responsibility                                                                         |
|---------------------------------|----------------------------------------------------------------------------------------|
| `internal/app`                  | Process wiring: dependency container, lifecycle, graceful shutdown                     |
| `internal/config`               | Runtime config loading and resolution (flags/env)                                      |
| `internal/contracts/methodmask` | Shared HTTP-method bitmask contract used by compiler and router                        |
| `internal/controlplane`         | Load → normalize → compile → validate → snapshot pipeline for routes/policies/services |
| `internal/dataplane/router`     | Radix trie route index: build, insert, lookup, header matching                         |
| `internal/dataplane/policy`     | Header mutation execution (request/response)                                           |
| `internal/dataplane/proxy`      | `Executor` — the terminal handler that performs the actual upstream call               |
| `internal/dataplane/request`    | Request normalization, hop-by-hop header stripping, forwarded-header handling          |
| `internal/dataplane/middleware` | Cross-cutting HTTP middleware (recovery, request ID, metrics, timeout)                 |
| `internal/dataplane/pipeline`   | Composes middleware chain + router + executor into the served handler                  |
| `internal/edge`                 | Listener-facing HTTP servers: public traffic and admin/system endpoints                |
| `internal/observe`              | Health checks and Prometheus metrics registration                                      |

## Roadmap / known limitations

This is a pre-1.0 project; the following is deliberately out of scope for the current milestone rather than overlooked:

- **Recovery and timeout middleware** are in progress, not yet wired into the default chain.
- **Metrics middleware** — the design uses a `sync.Pool`-backed `ResponseWriter` wrapper to capture status code and latency without allocating per request; route-pattern labeling (not raw path, to avoid unbounded Prometheus cardinality) is threaded from the executor back to the middleware via a context-carried recorder. Implementation is in progress.
- **Distributed tracing** is intentionally deferred. It requires an OpenTelemetry dependency, an exporter, and context propagation into the executor — real infrastructure, not just a middleware shim — and isn't justified before the gateway has more than one hop worth tracing.
- **gRPC upstreams** are not yet supported; the executor currently proxies HTTP only.

Config schemas and public interfaces may change before an initial stable release.
