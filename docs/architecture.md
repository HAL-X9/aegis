# Aegis Architecture

## Scope

This document defines the process architecture of Aegis, the mandatory package boundaries, and lifecycle guarantees for startup and shutdown. It is normative for process composition and runtime responsibilities. Detailed routing semantics and matching rules are specified in `docs/routing.md` and are not redefined here.

## Architectural Principles

Aegis is designed with strict separation between control-plane input, build-time dataplane preparation, and runtime request execution. Configuration loading, manifest parsing, and route compilation are cold-path operations and shall complete before any listener accepts traffic. Runtime request handling shall not perform configuration parsing or route compilation.

The router package provides the only supported runtime routing interface for the dataplane executor. Direct dependency of the proxy layer on trie internals, insertion logic, or control-plane route models is prohibited.

## Process Composition

Process composition is owned by `internal/app/program.go`. Program initialization resolves configuration sources, loads runtime settings, loads the gateway manifest, constructs long-lived dependencies, and initializes HTTP server coordination.

Dependency wiring is owned by `internal/app/registry.go`. Bootstrap validates required inputs and assembles the object graph for both traffic planes: a public HTTP server for client traffic and a system HTTP server for operational endpoints. Listener parameters, transport settings, and logging configuration are sourced from `internal/config`.

Server lifecycle orchestration is owned by `internal/app/servergroup.go`. Public and system servers execute concurrently under a shared cancellation context. A fatal run error in either server is treated as a process-level failure and triggers coordinated shutdown.

## Traffic Planes

The public plane serves user traffic through `internal/dataplane/proxy.Executor`. The executor depends on `internal/dataplane/router.Engine` for path candidate resolution and is responsible for request execution behavior on top of routing results.

The system plane serves process-local operational endpoints through `internal/edge/admin` with handlers in `internal/edge/admin/handlers`. This plane includes liveness reporting and shutdown visibility via `internal/observe/health`.

Public and system planes shall remain independently configurable and independently listenable through `listeners.public` and `listeners.system`.

## Routing Boundary Contract

Runtime routing state is encapsulated in `router.Engine`. Engine construction (`BuildEngine`) performs compile-and-build preparation from control-plane manifest data and publishes an immutable lookup structure for runtime use.

The proxy layer may call only engine-level lookup/match APIs. Router-internal representations, including trie nodes and insertion/indexing mechanics, are internal implementation details and shall not leak into proxy code or other runtime packages.

This contract is mandatory for architectural stability and allows routing index internals to evolve without requiring proxy-layer refactoring.

## Lifecycle Guarantees

Shutdown behavior is explicit and idempotent. During coordinated shutdown, public ingress is terminated before system endpoints are closed. Health state is updated to reflect shutdown intent before listener closure completes.

If server-group execution fails after startup, the process performs deterministic cancellation and close operations for all managed servers. Partial shutdown without coordinated close is not permitted.

## Configuration Boundary

`internal/config` defines process-local runtime configuration only: listener addresses and limits, listener timeout policy, outbound transport tuning, and logging configuration.

Gateway routing semantics are defined by the control-plane manifest and compilation pipeline, and are intentionally excluded from runtime process config. This separation is required to preserve a stable runtime contract and predictable reload/build behavior.
