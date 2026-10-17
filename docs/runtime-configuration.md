# Runtime Configuration

This document specifies the process-level configuration of Aegis: the file that defines listener addresses, timeouts, upstream connection parameters, and observability settings. The routing configuration (services, routes, and policies) is specified in [configuration.md](configuration.md).

The document describes the behavior of the implementation at the time of writing. Fields that are present in the schema but are not applied by the process are identified explicitly in Section 6.

---

## 1. Introduction

### 1.1 Scope

Aegis consumes two independent configuration documents. The **runtime configuration** determines how the process accepts inbound traffic, how it forwards traffic to upstream services, and how it reports its own state. The **routing configuration** (the gateway manifest) defines the rules that map requests to services. The runtime configuration is read once at process start. The routing configuration is compiled into an immutable snapshot that the request path consumes.

**Table 1.** Comparison of the two configuration documents.

| Property                       | Runtime                                                    | Routing (gateway)                                                               |
|--------------------------------|------------------------------------------------------------|---------------------------------------------------------------------------------|
| Default file in the repository | `configs/aegis.yaml`                                       | `configs/gateway.yaml`                                                          |
| Command-line flag              | `-config`                                                  | `-routes`                                                                       |
| Environment variable           | `AEGIS_RUNTIME_CONFIG_PATH`                                | `AEGIS_ROUTES_CONFIG_PATH`                                                      |
| Go type                        | `config.Runtime` ([types.go](../internal/config/types.go)) | `schema.GatewayConfig` ([config.go](../internal/controlplane/schema/config.go)) |
| Consumers                      | `app`, `edge`, the `proxy` transport                       | `controlplane`, then `snapshot`                                                 |
| Reload without restart         | not supported                                              | not supported                                                                   |
| Specification                  | this document                                              | [configuration.md](configuration.md)                                            |

### 1.2 Normative conventions

The key words "MUST", "MUST NOT", "SHOULD", and "MAY" are to be interpreted as described in RFC 2119. A requirement that is violated renders the configuration invalid or causes startup to fail. A recommendation may be departed from with justification. Source references are relative to the repository root.

---

## 2. Loading

Configuration is loaded in `app.New` ([app.go](../internal/app/app.go)). Loading consists of four sequential stages. An error in any stage aborts startup:

1. resolution of the file path (Section 2.1);
2. reading and decoding of the YAML document with strict field checking (Section 2.2);
3. semantic validation of the values (Section 2.3);
4. initialization of dependencies, including the loading of TLS certificates (Section 2.4).

`app.New` performs no network I/O. Listeners begin to accept connections only when `Run` is invoked.

### 2.1 Path resolution

The path to the runtime configuration is supplied in one of two ways. The command-line flag takes precedence over the environment variable:

1. the flag `-config`, if it is set to a non-empty value;
2. the environment variable `AEGIS_RUNTIME_CONFIG_PATH`.

If neither is supplied, the process terminates with an error. No default path is assumed ([resolve.go](../internal/config/resolve.go)). An environment variable that is set to an empty string is also an error. A relative path is resolved against the working directory of the process.

### 2.2 Decoding

The file is read in its entirety and decoded by `gopkg.in/yaml.v3` with the `KnownFields` option enabled ([yamlx.go](../internal/yamlx/yamlx.go)). Consequently, **any key that has no corresponding field in the Go type is an error**; it is not ignored. This behavior protects against typographical errors: a misspelled key such as `read_timout` produces a failure at startup rather than being silently discarded.

Only the first YAML document in the file is decoded. An empty file produces a decoding error whose message contains `EOF`.

The following output illustrates the error produced by an unknown key. The example uses a hypothetical misspelling:

```text
aegis: bootstrap: load runtime config (configs/aegis.yaml): failed to load app configuration from YAML:
decode config configs/aegis.yaml: yaml: unmarshal errors:
  line 14: field read_timout not found in type config.Timeouts
```

### 2.3 Validation

After decoding, `config.Validate` ([validate.go](../internal/config/validate.go)) is invoked. It validates the `listeners.public`, `listeners.system`, and `upstream_transport` sections and the `observability.logging` section. Other sections are not validated (Section 9). Validation returns the first error encountered.

### 2.4 Timing of application

Values take effect at one of two points:

- **during decoding and validation** (`config.Load`): the format of addresses, the ranges of timeouts, and the permitted enumerations are checked;
- **during initialization** (`app.Bootstrap`): TLS certificates are loaded and the HTTP servers are constructed. Errors concerning the contents of certificate files are detected only at this stage, even when the configuration has passed validation.

### 2.5 Default values

Defaults are defined in one place, `DefaultRuntime` ([defaults.go](../internal/config/defaults.go)). The loader decodes the document on top of that value, so an omitted field keeps its default, and a field that is present always wins, including an explicit zero: `request_timeout: 0` disables the request deadline instead of falling back to 30 s. Defaults are validated together with the rest of the document.

A field has a default only when a safe, bounded value exists. These fields have none and keep the zero value of their Go type:

- `listeners.*.addr`, which is required;
- `listeners.*.tls`, whose absence means plain HTTP;
- `proxy.trusted_proxies`, whose absence trusts no address;
- `observability.tracing.*`, which is off by default;
- `upstream_transport.max_conns_per_host`, where 0 means no limit.

The defaults equal the values in [configs/aegis.yaml](../configs/aegis.yaml); they are listed per field in Sections 4 and 5. The requirement that a field be present is not always expressed by validation: for some fields the zero value is permitted and denotes "no limit"; for others it is rejected.

---

## 3. Document structure

```yaml
listeners:
  public:              # public listener (client traffic)
    addr: string
    tls:               # optional; when present, HTTPS is enabled
      cert_file: string
      key_file: string
    timeouts:
      read_timeout: duration
      read_header_timeout: duration
      write_timeout: duration
      idle_timeout: duration
      request_timeout: duration
    max_header_bytes: int
  system:              # system listener (health and metrics)
    addr: string
    tls: { ... }
    timeouts: { ... }
    max_header_bytes: int

upstream_transport:    # connection parameters for upstream services
  max_idle_conns: int
  max_idle_conns_per_host: int
  max_conns_per_host: int
  dial_timeout: duration
  tls_handshake_timeout: duration
  response_header_timeout: duration
  idle_conn_timeout: duration

proxy:                 # reserved (Section 6.1)
  trusted_proxies: [ CIDR ]

middleware:            # optional layers of the public listener (Section 4.2)
  metrics: bool
  request_id: bool

observability:         # partially reserved (Sections 6.2 and 6.3)
  logging:
    level: enum
    format: enum
  tracing:
    enabled: bool
    endpoint: string
```

Durations are written as strings accepted by `time.ParseDuration`, for example `500ms`, `10s`, or `2m`.

---

## 4. Listeners (`listeners`)

The process starts two independent HTTP servers. The public listener serves client traffic through routing and the proxy. The system listener serves operational endpoints and does not pass requests through routing. The server for each listener is constructed by [`NewPublicServer`](../internal/edge/public/server.go) or [`NewSystemServer`](../internal/edge/admin/server.go).

### 4.1 Common HTTP listener fields

Both listeners share the same schema. Validation is performed by `validateHTTP` ([validate.go](../internal/config/validate.go)).

**Table 2.** HTTP listener fields.

| Field                          | Type                | Value when omitted           | Validation at load time                               | Semantics                                                                                                  |
|--------------------------------|---------------------|------------------------------|-------------------------------------------------------|------------------------------------------------------------------------------------------------------------|
| `addr`                         | string, `host:port` | not permitted                | `net.SplitHostPort`                                   | The address passed to `net.Listen`. An empty host is permitted (`":8080"`) and denotes all interfaces.     |
| `tls`                          | object              | plain HTTP                   | when present, `cert_file` and `key_file` are required | Enables HTTPS (Section 4.4).                                                                               |
| `timeouts.read_timeout`        | duration            | `10s` public, `5s` system    | at least 1 ms                                         | Bounds the reading of the entire request, including the body.                                              |
| `timeouts.read_header_timeout` | duration            | `5s` public, `2s` system     | not greater than `read_timeout`                       | Bounds the reading of request headers.                                                                     |
| `timeouts.write_timeout`       | duration            | `30s` public, `10s` system   | not negative                                          | Bounds the writing of the response.                                                                        |
| `timeouts.idle_timeout`        | duration            | `120s` public, `30s` system  | not validated                                         | Bounds the wait for the next request on a keep-alive connection.                                           |
| `timeouts.request_timeout`     | duration            | `30s` (public)               | not validated                                         | Deadline for the processing of a request in middleware. Applies to the public listener only (Section 4.2). |
| `max_header_bytes`             | integer             | 1 MiB public, 256 KiB system | not negative                                          | Maximum size of the request headers.                                                                       |

The field `addr` is the only one without a default. An explicit zero `read_timeout` is rejected by validation. The field `read_header_timeout` MUST NOT exceed `read_timeout`; since both have defaults, overriding `read_timeout` alone to a value below the default `read_header_timeout` is rejected, and the two should be set together. An explicit `0` in the other timeout fields keeps its `net/http` meaning, given in the table as the behavior of a zero value.

### 4.2 Public listener (`listeners.public`)

The public listener accepts client requests. Each request traverses a middleware chain assembled by `NewPublicServer` ([server.go](../internal/edge/public/server.go)). The layers, from outermost to innermost, are:

1. `timeout`, which applies `request_timeout`;
2. `metrics`, which records the count and duration of requests (optional);
3. `request_id`, which ensures the presence of an `X-Request-Id` header (optional);
4. the routing handler, which passes the request to `proxy.Executor`.

The two optional layers are switched by the top-level section `middleware`:

**Table 2a.** Fields of `middleware`.

| Field        | Type | Value when omitted | Effect of `false`                                                                                                                                                                                                           |
|--------------|------|--------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `metrics`    | bool | `true`             | The `metrics` layer is not installed and the HTTP metrics collector is neither created nor registered, so no `aegis_http_*` series exist. The endpoint `GET /metrics` of the system listener (Section 4.3) is not affected. |
| `request_id` | bool | `true`             | The `request_id` layer is not installed: no ID is generated, a client-supplied `X-Request-Id` is forwarded to the upstream unchecked, and the header is not echoed in the response.                                         |

The whole section, or either key, may be omitted; an omitted setting is enabled. The section applies to the public listener only: the system listener has no middleware. The order of the layers is fixed and is not configurable; the timeout is controlled by `request_timeout` and has no switch here. An unknown key under `middleware` is rejected at load time like any other unknown field.

The field `request_timeout` specifies the upper bound on the processing time of a single request, including retries to the upstream. It is implemented as a context deadline on the request. When it expires before the upstream has responded, the client receives `504 Gateway Timeout`. Once the response headers have been sent the status can no longer change, and an expiry while the body is streaming truncates the response. The default is 30 s. A value of 0, or a negative value, disables the bound ([timeout.go](../internal/dataplane/middleware/timeout.go)).

The service timeout (`timeout` in the routing manifest; see [configuration.md](configuration.md#timeouts-and-retries)) and `request_timeout` apply concurrently: the deadline that expires first determines the outcome. Validation does not relate the two values. If `request_timeout` is shorter than the service timeout, the service timeout is never reached, because the request terminates earlier.

### 4.3 System listener (`listeners.system`)

The system listener serves three endpoints ([router.go](../internal/edge/admin/router.go)):

| Method and path | Purpose                                     | Response                                       |
|-----------------|---------------------------------------------|------------------------------------------------|
| `GET /livez`    | liveness of the process                     | `200 ok` whenever the process is running       |
| `GET /readyz`   | readiness to accept traffic                 | `503` once shutdown has begun; otherwise `200` |
| `GET /metrics`  | metrics in the Prometheus exposition format | —                                              |

The system listener applies no middleware and does not use the field `request_timeout`. The fields `timeouts` and `max_header_bytes` apply in the same manner as for the public listener.

The system listener SHOULD be bound to an internal interface. The value `0.0.0.0:18080` in [configs/aegis.yaml](../configs/aegis.yaml) exposes the metrics endpoint on every interface of the host. In deployments beyond a local environment, the address SHOULD be stated explicitly, for example `127.0.0.1:18080` or an address within an internal network.

### 4.4 TLS

The section `tls` enables HTTPS for a listener ([tls.go](../internal/config/tls.go)). Its presence requires that both fields be supplied:

```yaml
tls:
  cert_file: /etc/aegis/tls/server.crt
  key_file:  /etc/aegis/tls/server.key
```

The certificate and key are loaded once during initialization by `tls.LoadX509KeyPair`. The minimum protocol version is TLS 1.2. Client certificate verification is not configurable. The certificate is not reloaded while the process runs; renewing a certificate requires a restart.

When `tls` is omitted, the listener operates over plain HTTP.

---

## 5. Upstream transport (`upstream_transport`)

This section parameterizes the `http.Transport` through which `proxy.Executor` performs all requests to upstream services ([app.go](../internal/app/app.go), function `newUpstreamTransport`). There is one transport per process, shared by all routes and services. Parameters cannot be overridden for an individual service.

**Table 3.** Fields of the `upstream_transport` section.

| Field                     | Type     | Value 0       | Validation at load time                                            | Semantics                                                                                                                                                                                                                  |
|---------------------------|----------|---------------|--------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `max_idle_conns`          | integer  | no limit      | not negative                                                       | Total number of idle connections across the pool.                                                                                                                                                                          |
| `max_idle_conns_per_host` | integer  | not permitted | at least 1                                                         | Number of idle connections per host. In `net/http`, a value of 0 denotes 2, which causes repeated connection establishment under concurrent load; 0 is therefore rejected ([validate.go](../internal/config/validate.go)). |
| `max_conns_per_host`      | integer  | no limit      | not negative; if positive, not less than `max_idle_conns_per_host` | Upper bound on open connections per host.                                                                                                                                                                                  |
| `dial_timeout`            | duration | no limit      | not validated                                                      | Timeout for establishing a TCP connection.                                                                                                                                                                                 |
| `tls_handshake_timeout`   | duration | no limit      | not validated                                                      | Timeout for the TLS handshake with the upstream.                                                                                                                                                                           |
| `response_header_timeout` | duration | no limit      | not validated                                                      | Time to wait for response headers after the request is sent. When it expires, the client receives `504`.                                                                                                                   |
| `idle_conn_timeout`       | duration | no limit      | not validated                                                      | Lifetime of an idle connection in the pool.                                                                                                                                                                                |

Three parameters are fixed in the implementation and cannot be configured:

- `ExpectContinueTimeout`: 1 s;
- `KeepAlive` for connection establishment: 30 s;
- `ForceAttemptHTTP2`: `true`, meaning that HTTP/2 is used when the upstream supports it.

The defaults are: `max_idle_conns` 1024, `max_idle_conns_per_host` 1024, `dial_timeout` 3 s, `tls_handshake_timeout` 5 s, `response_header_timeout` 10 s, `idle_conn_timeout` 90 s; `max_conns_per_host` has none (0, no limit). The rule that `max_idle_conns_per_host` MUST be at least 1 applies to an explicit value only, since the field is no longer left at 0 by omission.

An explicit duration of 0 denotes the absence of a limit in the sense of `net/http`, so setting `dial_timeout` or `response_header_timeout` to 0 allows an indefinite wait for an upstream.

Negative durations are not rejected by validation.

---

## 6. Reserved sections

The fields in this section are present in the schema and in [configs/aegis.yaml](../configs/aegis.yaml), but **they do not affect the behavior of the process**. Some of them are validated; the result of validation is not used.

### 6.1 `proxy.trusted_proxies`

The field is decoded into `config.Proxy` ([types.go](../internal/config/types.go)) but is read nowhere. The `X-Forwarded-For` header is derived from the address of the connection (`RemoteAddr`) and **always replaces** any value supplied by the client ([header.go](../internal/dataplane/request/header.go)). The list of trusted proxies is neither checked nor validated as CIDR notation.

The field will become meaningful once a trusted-address chain is implemented. Until then, it SHOULD either be removed from the file or accompanied by a comment stating that it has no effect.

### 6.2 `observability.logging`

The fields `level` and `format` default to `info` and `json` and are validated: `level` MUST be one of `debug`, `info`, `warn`, `error`, `dpanic`, `panic`, or `fatal`; `format` MUST be `json` or `console`. The process, however, writes its log through the standard `log` package with the prefix `aegis:`. Neither value is applied to any component.

The validation error message refers to the key `logging_config.level`, whereas the key in the schema is `observability.logging.level`. This inconsistency in the message text is to be corrected.

### 6.3 `observability.tracing`

The fields `enabled` and `endpoint` are decoded but neither validated nor used. Distributed tracing is not implemented (see the roadmap in [ARCHITECTURE.md](../ARCHITECTURE.md)). The setting `enabled: true` has no effect.

---

## 7. Startup errors

Table 4 lists the errors most likely to be encountered. Message fragments are quoted from the implementation; prefixes added by the wrapping of errors are omitted.

**Table 4.** Startup errors related to the runtime configuration.

| Message fragment                                                          | Cause                                                             | Remedy                                                            |
|---------------------------------------------------------------------------|-------------------------------------------------------------------|-------------------------------------------------------------------|
| `AEGIS_RUNTIME_CONFIG_PATH is not set and --config is empty`              | the path is supplied neither by flag nor by variable              | pass `-config` or set the variable                                |
| `AEGIS_RUNTIME_CONFIG_PATH is set but empty`                              | the variable is set to an empty string                            | provide a path or unset the variable                              |
| `field <name> not found in type config.<Type>`                            | an unknown key: a typographical error or a key outside the schema | correct the name or remove the key                                |
| `invalid addr`                                                            | `addr` is not in `host:port` form                                 | use a form such as `:8080` or `127.0.0.1:8080`                    |
| `read_timeout must be at least 1ms`                                       | `read_timeout` is omitted or less than 1 ms                       | set a value such as `10s`                                         |
| `read_header_timeout cannot exceed read_timeout`                          | `read_header_timeout` is greater than `read_timeout`              | reduce `read_header_timeout`                                      |
| `write_timeout cannot be negative`, `max_header_bytes cannot be negative` | a negative value                                                  | supply a non-negative value                                       |
| `tls: cert_file is required`, `tls: key_file is required`                 | the `tls` section is present but a field is empty                 | supply both paths                                                 |
| `max_idle_conns_per_host must be >= 1`                                    | the value is 0 or omitted                                         | set 1 or greater                                                  |
| `max_idle_conns_per_host cannot exceed max_conns_per_host`                | the idle limit exceeds the total limit                            | reduce `max_idle_conns_per_host` or increase `max_conns_per_host` |
| `logging_config.level must be one of: ...`                                | an unsupported log level                                          | select a value from the list (Section 6.2)                        |
| `load certificate "..." and key "...": ...`                               | certificate files are missing or do not form a matching pair      | verify the paths and the certificate/key pair                     |

The last error occurs during initialization, not during validation. A configuration may therefore pass validation and still fail to start.

---

## 8. Complete example

The example conforms to the current schema. Keys described in Section 6 are included and annotated, because they appear in [configs/aegis.yaml](../configs/aegis.yaml).

```yaml
listeners:
  public:
    addr: ":8080"                     # all interfaces, port 8080
    timeouts:
      read_timeout: 10s               # reading of the entire request
      read_header_timeout: 5s         # reading of headers; not greater than read_timeout
      write_timeout: 30s              # writing of the response
      idle_timeout: 120s              # wait for the next request on a keep-alive connection
      request_timeout: 30s            # deadline for request processing, including retries
    max_header_bytes: 1048576         # 1 MiB
    # tls:                            # uncomment to enable HTTPS
    #   cert_file: /etc/aegis/tls/server.crt
    #   key_file:  /etc/aegis/tls/server.key

  system:
    addr: "0.0.0.0:18080"            # /livez, /readyz, /metrics; see Section 4.3
    timeouts:
      read_timeout: 5s
      read_header_timeout: 2s
      write_timeout: 10s
      idle_timeout: 30s
    max_header_bytes: 262144          # 256 KiB

upstream_transport:
  max_idle_conns: 1024
  max_idle_conns_per_host: 1024
  max_conns_per_host: 0               # no limit
  dial_timeout: 3s
  tls_handshake_timeout: 5s
  response_header_timeout: 10s        # expiry yields 504
  idle_conn_timeout: 90s

proxy:                                # reserved; Section 6.1
  trusted_proxies:
    - 10.0.0.0/8
    - 172.16.0.0/12
    - 192.168.0.0/16

middleware:                           # optional layers of the public listener; Section 4.2
  metrics: true                       # request count and duration; false: no aegis_http_* series
  request_id: true                    # X-Request-Id: keep a valid client ID or generate one

observability:                        # reserved; Sections 6.2 and 6.3
  logging:
    level: info
    format: json
  tracing:
    enabled: false
    endpoint: "http://localhost:4318"
```

---

## 9. Status of fields

Table 5 records the status of each field. It is intended as a reference for verification when the schema changes.

**Table 5.** Status of runtime configuration fields.

| Field                                                    | Validated             | Applied                | Status                              |
|----------------------------------------------------------|-----------------------|------------------------|-------------------------------------|
| `listeners.{public,system}.addr`                         | yes                   | yes                    | active                              |
| `listeners.{public,system}.tls`                          | presence of fields    | yes, at initialization | active                              |
| `listeners.{public,system}.timeouts.read_timeout`        | yes                   | yes                    | active                              |
| `listeners.{public,system}.timeouts.read_header_timeout` | yes                   | yes                    | active                              |
| `listeners.{public,system}.timeouts.write_timeout`       | yes                   | yes                    | active                              |
| `listeners.{public,system}.timeouts.idle_timeout`        | no                    | yes                    | active                              |
| `listeners.public.timeouts.request_timeout`              | no                    | yes (middleware)       | active for the public listener only |
| `listeners.system.timeouts.request_timeout`              | no                    | no                     | **ignored**                         |
| `listeners.{public,system}.max_header_bytes`             | yes                   | yes                    | active                              |
| `upstream_transport.*`                                   | partially (Section 5) | yes                    | active                              |
| `middleware.metrics`                                     | n/a (boolean)         | yes                    | active; default `true`              |
| `middleware.request_id`                                  | n/a (boolean)         | yes                    | active; default `true`              |
| `proxy.trusted_proxies`                                  | no                    | no                     | **reserved**                        |
| `observability.logging.level`                            | yes                   | no                     | **reserved**                        |
| `observability.logging.format`                           | yes                   | no                     | **reserved**                        |
| `observability.tracing.enabled`                          | no                    | no                     | **reserved**                        |
| `observability.tracing.endpoint`                         | no                    | no                     | **reserved**                        |

The field `listeners.system.timeouts.request_timeout` exists in the type because the `Timeouts` structure is shared by both listeners. Its presence in the system section has no effect.

---

## 10. Limitations

1. **No hot reload.** A change to any field requires a process restart, including a change to TLS certificates.
2. **Incomplete validation.** The sign of duration values, `idle_timeout`, `request_timeout`, `proxy.*`, and `observability.tracing.*` are not validated. The relationship between `request_timeout` and service timeouts is not validated either.
3. **Part of the schema is not applied** (Section 6). Values are accepted without warning, which may create the false impression that they have an effect.
4. **No environment-specific overrides.** Each environment uses its own file. Variable substitution within the YAML is not implemented.
5. **Few tests for the `internal/config` package.** Only the loading, the defaults (`DefaultRuntime`) and the `middleware` section are tested directly; the validation rules are verified indirectly through end-to-end tests (see [testing.md](testing.md)).

---

## 11. Related documents and source code

| Topic                           | Document                                                  | Source                                                                                 |
|---------------------------------|-----------------------------------------------------------|----------------------------------------------------------------------------------------|
| Path resolution and loading     | this document, Section 2                                  | [resolve.go](../internal/config/resolve.go), [app.go](../internal/app/app.go)          |
| Types and validation            | this document, Sections 4 to 6                            | [types.go](../internal/config/types.go), [validate.go](../internal/config/validate.go) |
| TLS                             | Section 4.4                                               | [tls.go](../internal/config/tls.go)                                                    |
| Lifecycle and shutdown          | [ARCHITECTURE.md](../ARCHITECTURE.md#process-lifecycle)   | [lifecycle.go](../internal/app/lifecycle.go)                                           |
| Service timeouts and retries    | [configuration.md](configuration.md#timeouts-and-retries) | [retry.go](../internal/dataplane/retry/retry.go)                                       |
| Routing errors and status codes | [routing-and-errors.md](routing-and-errors.md)            | [executor.go](../internal/dataplane/proxy/executor.go)                                 |
