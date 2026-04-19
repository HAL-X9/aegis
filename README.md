# Aegis

**One runtime for routing, policy, proxy, and observability.**

Aegis is an edge-oriented execution environment: declarative configuration defines how inbound traffic is matched, authorized or constrained, forwarded to upstream services, and observed. The same binary is intended to run on bare metal, in containers, and in orchestrated environments without changing the core semantics of routing, policy evaluation, and proxy behavior.

The project is under active development; public interfaces and configuration schemas may evolve until an initial stable release.

---

## Quick start

This section describes the minimum path from a clean clone to a running process on your workstation. It assumes a supported Go toolchain and network access only for module resolution on first build.

### Prerequisites

| Requirement | Notes |
| --- | --- |
| Go | Version **1.26.1** or newer (see `go.mod`). |
| Runtime configuration | A valid YAML file conforming to the runtime schema (example: `configs/aegis.yaml`). |

### Run (local development)

1. Clone the repository and change to the repository root.
2. Resolve dependencies (optional if modules are already cached):

   ```bash
   go mod download
   ```

3. Start the process with an explicit configuration path:

   ```bash
   go run ./cmd -config configs/aegis.yaml
   ```

   The process binds to the address declared in the runtime file (`http.addr`; the sample configuration listens on **`:8080`**).

#### Alternative: environment-based configuration

If the `-config` flag is omitted, the binary resolves the runtime file from the environment variable `AEGIS_RUNTIME_CONFIG_PATH`. The flag takes precedence when both are set.

| Input | Resolution order |
| --- | --- |
| `-config <path>` | Used as the runtime configuration path. |
| `AEGIS_RUNTIME_CONFIG_PATH` | Used when `-config` is not provided. |
| Neither set | The process exits at startup with a descriptive error. |

Example:

```bash
export AEGIS_RUNTIME_CONFIG_PATH=configs/aegis.yaml
go run ./cmd
```

### Verify the listener

With the sample configuration, confirm that the HTTP server accepts connections (for example, from a second terminal):

```bash
curl -i http://127.0.0.1:8080/livez
```

Liveness responds on **`GET /livez`**. Other paths return **`404 Not Found`** until dataplane routes are registered. A successful TCP connection and an HTTP response from the process indicate that the listener is up as configured.

For a non-HTTP check of the listening socket:

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
```

### Production-oriented execution

For deployment outside ad-hoc development, build a static binary from the repository root and invoke it with the same configuration contract:

```bash
go build -o aegis ./cmd
./aegis -config /path/to/runtime.yaml
```

Run the binary under your platform’s process supervisor or container entrypoint; ensure `AEGIS_RUNTIME_CONFIG_PATH` or `-config` is set consistently with your release artifact and configuration management practices.
