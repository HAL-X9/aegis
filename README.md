# Aegis

A high-performance HTTP API gateway written in Go — routing, policy enforcement, proxying, and observability, built around a strict separation between control plane and data plane.

Configuration (routes, services, policies) is compiled once at startup into an immutable snapshot. The request path never parses or validates configuration: route lookup walks a precompiled radix trie without allocating, and everything that follows — method and header checks, header mutations, rate limiting, retries — executes precompiled plans.

## Why

Configuration is treated as a build artifact rather than something interpreted at request time. The control plane loads, validates, normalizes, and compiles YAML into a snapshot; the data plane only ever executes against that compiled snapshot. Adding a route, a policy, or a service changes what the compiler produces — it never changes the request-serving code path itself.

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full request lifecycle and the reasoning behind each design decision.

## Quick start

### Run with Docker

```bash
git clone https://github.com/HAL-X9/aegis.git
cd aegis
make up
```

### Run from source

```bash
git clone https://github.com/HAL-X9/aegis.git
cd aegis
go mod download
make run
```

The sample routes target the host `mock-upstream`, which resolves only inside Docker Compose. When running from source, map it to a local address and start the stand-in; see [docs/testing.md](docs/testing.md), Section 10.2.

The public listener serves client traffic on `:8080`. The system listener serves `/livez`, `/readyz`, and `/metrics` on `:18080`.

### Configuration sources

Aegis resolves each configuration path from its CLI flag first, then from its environment variable. If neither is set, startup fails with an explicit error rather than falling back to a hidden default.

| Config  | Flag      | Env var                     |
|---------|-----------|-----------------------------|
| Runtime | `-config` | `AEGIS_RUNTIME_CONFIG_PATH` |
| Routes  | `-routes` | `AEGIS_ROUTES_CONFIG_PATH`  |

```bash
export AEGIS_RUNTIME_CONFIG_PATH=configs/aegis.yaml
export AEGIS_ROUTES_CONFIG_PATH=configs/gateway.yaml
go run ./cmd
```

### Production build

```bash
make build
./bin/aegis -config /path/to/aegis.yaml -routes /path/to/gateway.yaml
```

## Testing & benchmarks

```bash
make test
make test-race
make fuzz
make bench
make bench-proxy
```

## Status

Aegis is pre-1.0 and under active development. Public interfaces and configuration schemas may change.

## Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) — design decisions, request lifecycle, package layout
- [docs/configuration.md](docs/configuration.md) — configuration model and validation rules
- [docs/routing.md](docs/routing.md) — routing engine architecture
- [docs/routing-and-errors.md](docs/routing-and-errors.md) — route matching semantics and the error-response contract
- [docs/policies.md](docs/policies.md) — policy engine and header mutations
- [docs/runtime-configuration.md](docs/runtime-configuration.md) — process-level configuration: listeners, TLS, timeouts, upstream transport
- [docs/testing.md](docs/testing.md) — test layers, fuzzing, benchmarks, linting, coverage and manual checks