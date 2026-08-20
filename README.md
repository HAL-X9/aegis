# Aegis

A high-performance HTTP API gateway written in Go — routing, policy enforcement, proxying, and observability, built around a strict separation between control plane and data plane.

Configuration (routes, services, policies) is compiled once at startup into immutable, allocation-free lookup structures. The request hot path never parses config, never touches a map with string-building keys, and never allocates on the common case — it only walks a precompiled radix trie and dispatches.

```
BenchmarkLookupHighFanout/routes=8192-10     12,976,374    91.07 ns/op    0 B/op    0 allocs/op
BenchmarkLookupDeep/routes=4096-10            8,824,230   132.10 ns/op    0 B/op    0 allocs/op
BenchmarkLookupMixed/routes=12288-10          8,213,004   145.60 ns/op    0 B/op    0 allocs/op
```
High-fanout route lookup stays below 100ns across 16–32,768 routes, with 0 allocations per operation. Deep and mixed-path lookups (interleaved static/param/wildcard routes) stay allocation-free as well, with latency in the ~116–146ns range across the tested route counts. Full benchmark suite in internal/dataplane/router.

## Why

Most gateway tutorials route with `map[string]http.Handler` and re-derive behavior from config on every request. Aegis instead treats configuration as a **build artifact**: control plane loads, normalizes, validates, and compiles YAML into a snapshot; data plane only ever executes against that compiled snapshot. See [ARCHITECTURE.md](ARCHITECTURE.md) for the full request lifecycle and the reasoning behind each design decision.

## Quick start

### Run from source

```bash
git clone https://github.com/HAL-X9/aegis.git
cd aegis
go mod download
go run ./cmd -config configs/aegis.yaml -routes configs/gateway.yaml
```

### Run with Docker

```bash
git clone https://github.com/HAL-X9/aegis.git
cd aegis
docker compose up -d --build
```

### Configuration sources

Aegis resolves config paths from CLI flags first, then environment variables. If neither is set, startup fails with an explicit error rather than falling back to a hidden default.

| Config  | Flag      | Env var                     |
|---------|-----------|-----------------------------|
| Runtime | `-config` | `AEGIS_RUNTIME_CONFIG_PATH` |
| Routes  | `-routes` | `AEGIS_ROUTES_CONFIG_PATH`  |

```bash
export AEGIS_RUNTIME_CONFIG_PATH=configs/aegis.yaml
export AEGIS_ROUTES_CONFIG_PATH=configs/gateway.yaml
go run ./cmd
```

### Verify it's running

```bash
curl -i http://127.0.0.1:18080/livez
```

Public traffic is served on `:8080` and routed through the data plane using `configs/gateway.yaml`. The system plane (health, metrics) listens separately on `:18080`, so operational endpoints are never exposed on the same surface as user traffic.

### Production build

```bash
go build -o app ./cmd
./app -config /path/to/aegis.yaml -routes /path/to/gateway.yaml
```

## Testing & benchmarks

```bash
go test ./...
go test ./internal/dataplane/router/ -bench . -benchmem
```

## Status

Aegis is pre-1.0 and under active development. Public interfaces and config schemas may change.

## Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) — design decisions, request lifecycle, package layout
- [docs/policies.md](docs/policies.md) — policy engine and header mutations
