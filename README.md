# Aegis

**Gateway — one runtime for routing, policy, proxy, and observability.**

Aegis is an edge-oriented execution environment: declarative configuration defines how inbound traffic is matched, authorized or constrained, forwarded to upstream services, and observed. The same binary is intended to run on bare metal, in containers, and in orchestrated environments without changing the core semantics of routing, policy evaluation, and proxy behavior.

The project is under active development; public interfaces and configuration schemas may evolve until an initial stable release.


## Quick start

This section describes the minimum path from a clean clone to a running process on your workstation. It assumes a supported Go toolchain and network access only for module resolution on first build.

### Prerequisites

Before you run Aegis locally, make sure:

- Go **1.26.0+** is installed (`go.mod` is the source of truth).
- You have a valid runtime YAML config (for example, `configs/aegis.yaml`).
- You have a routes manifest (for example, `configs/gateway.yaml`).

### Run (local development)

1. Clone the repository and change to the repository root.
2. Resolve dependencies (optional if modules are already cached):

   ```bash
   go mod download
   ```

3. Start the process with explicit runtime and routes configuration paths:

   ```bash
   go run ./cmd -config configs/aegis.yaml -routes configs/gateway.yaml
   ```

   The process starts two listeners declared in the runtime file: public on **`:8080`** and system on **`127.0.0.1:18080`** in the sample configuration.

#### Alternative: environment-based configuration

If CLI flags are omitted, Aegis resolves config paths from environment variables.
When both are provided, **CLI flags win**.

Path resolution is straightforward: for runtime config, Aegis uses `-config` first and falls back to `AEGIS_RUNTIME_CONFIG_PATH`; for routes config, it uses `-routes` first and falls back to `AEGIS_ROUTES_CONFIG_PATH`. If neither source is set for runtime or routes, startup fails with a clear error.

Example (env-only startup):

```bash
export AEGIS_RUNTIME_CONFIG_PATH=configs/aegis.yaml
export AEGIS_ROUTES_CONFIG_PATH=configs/gateway.yaml
go run ./cmd
```

### Verify the listener

With the sample configuration, verify the system listener liveness endpoint:

```bash
curl -i http://127.0.0.1:18080/livez
```

Liveness responds on **`GET /livez`** from the system plane. Public traffic is served on **`:8080`** and forwarded through the dataplane using routes from `configs/gateway.yaml`.

For a non-HTTP check of the listening socket:

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
lsof -nP -iTCP:18080 -sTCP:LISTEN
```

### Production-oriented execution

For deployment outside ad-hoc development, build a static binary from the repository root and invoke it with the same configuration contract:

```bash
go build -o app ./cmd
./app -config /path/to/aegis.yaml -routes /path/to/gateway.yaml
```

Run the binary under your platform’s process supervisor or container entrypoint; ensure `AEGIS_RUNTIME_CONFIG_PATH`/`-config` and `AEGIS_ROUTES_CONFIG_PATH`/`-routes` are set consistently with your release artifact and configuration management practices.
