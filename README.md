# Aegis

A high-performance HTTP API gateway written in Go — routing, policy enforcement, proxying, and observability, built around a strict separation between control plane and data plane.

Configuration (routes, services, policies) is compiled once at startup into an immutable, allocation-free lookup structure. The request hot path never parses configuration, never touches a map with string-built keys, and never allocates on the common case — it only walks a precompiled radix trie and dispatches.

## Why

Configuration is treated as a build artifact rather than something interpreted at request time. The control plane loads, normalizes, validates, and compiles YAML into a snapshot; the data plane only ever executes against that compiled snapshot. Adding a route, a policy, or a service changes what the compiler produces — it never changes the request-serving code path itself.

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full request lifecycle and the reasoning behind each design decision.

## Lookup performance

| Benchmark                   | Routes | Latency      | Allocations         |
|-----------------------------|--------|--------------|---------------------|
| `BenchmarkLookupHighFanout` | 8,192  | 91.07 ns/op  | 0 B/op, 0 allocs/op |
| `BenchmarkLookupDeep`       | 4,096  | 132.10 ns/op | 0 B/op, 0 allocs/op |
| `BenchmarkLookupMixed`      | 12,288 | 145.60 ns/op | 0 B/op, 0 allocs/op |

Route lookup stays under 100 ns for high-fanout route sets and in the 130–150 ns range for deep and mixed static/param/wildcard paths, with zero allocations across the tested route counts. The full benchmark suite lives in `internal/dataplane/router`.

## Quick start

### Run with Docker

```bash
git clone https://github.com/HAL-X9/aegis.git
cd aegis
docker compose up -d --build
```

### Run from source

```bash
git clone https://github.com/HAL-X9/aegis.git
cd aegis
go mod download
go run ./cmd -config configs/aegis.yaml -routes configs/gateway.yaml
```

### Configuration sources

Aegis resolves configuration paths from CLI flags first, then from environment variables. If neither is set, startup fails with an explicit error rather than falling back to a hidden default.

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

Aegis is pre-1.0 and under active development. Public interfaces and configuration schemas may change.

## Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) — design decisions, request lifecycle, package layout
- [docs/policies.md](docs/policies.md) — policy engine and header mutations