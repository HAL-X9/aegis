# Testing

This document describes the testing strategy of Aegis: the layers of verification, their location and coverage, the conventions adopted in the repository, the procedures for running tests, benchmarks, and static analysis, and the known gaps. Coverage figures were obtained by measurement on the state of the code dated 2026-10-14 and are rounded.

---

## 1. Introduction

### 1.1 Objectives

The test suite is intended to establish three properties:

1. **Correctness of compilation and validation.** An invalid configuration is rejected before any traffic is processed. The rules are specified in [configuration.md](configuration.md) and [policies.md](policies.md).
2. **Correctness of routing and policy execution.** Priority order, status-code semantics, and the application of header policies conform to [routing-and-errors.md](routing-and-errors.md).
3. **Absence of avoidable allocations on the hot path.** Selected components are accompanied by tests that assert zero allocations per request.

### 1.2 Normative conventions

The key words "MUST" and "SHOULD" are interpreted as described in RFC 2119, consistently with [runtime-configuration.md](runtime-configuration.md).

---

## 2. Commands

All commands are run from the repository root. Each row gives the direct `go` invocation and, where one exists, the equivalent `Makefile` target.

**Table 1.** Execution commands.

| Task | Command | Makefile target |
|---|---|---|
| All tests | `go test ./...` | `make test` |
| All tests with the race detector | `go test ./... -race` | `make test-race` |
| Coverage by package | `go test ./... -cover` | `make coverage` |
| A single test with verbose output | `go test ./tests/e2e -run TestGateway_ServiceTimeout -v` | — |
| End-to-end tests without caching | `go test ./tests/... -count=1 -v` | — |
| Router benchmarks | `go test ./internal/dataplane/router/ -bench . -benchmem` | `make bench` |
| Static analysis | `golangci-lint run ./...` | `make lint` |
| Vet | `go vet ./...` | `make vet` |
| Formatting | `gofmt -w .` | `make fmt` |

When results are to be reproduced, the flag `-count=1` is recommended. Without it, the results of unchanged tests are taken from the build cache.

---

## 3. Layers of testing

The tests are organized in four layers. Each subsequent layer exercises more components and approximates the behavior of the running process more closely.

**Table 2.** Layers of testing.

| Layer | Location | Scope | Number of tests | Number of benchmarks |
|---|---|---|---|---|
| Unit | `internal/**/*_test.go` | the internal logic of one package | 85 | 9 |
| Integration | `tests/integration/` | routing and policies through the exported interfaces of the packages | 8 | — |
| End-to-end | `tests/e2e/` | the complete path from YAML to the HTTP response | 7 | — |
| Benchmarks | `internal/**/*_test.go` (`Benchmark*`) | the cost of the hot path | — | 17 (total) |

The total number of `Test*` functions is 100; the total number of `Benchmark*` functions is 17. Subtests created with `t.Run` are not included in these figures.

The integration and end-to-end tests require no external services. The end-to-end tests host the upstream stand-in in memory through `httptest` and use no fixed ports. Containers, Docker Compose, and the `mock/` directory are not required for the tests.

---

## 4. Unit tests

Unit tests reside alongside the code they verify and may reference unexported identifiers. Table 3 lists the packages that contain tests and the scope of each.

**Table 3.** Unit tests by package.

| Package | Tests | Benchmarks | Scope |
|---|---|---|---|
| `internal/controlplane/validate` | 41 | — | rules for paths, methods, headers, services, policies, cross-section references, retry bounds, and the ranges of `rate` and `burst` |
| `internal/controlplane/compile` | 17 | — | construction of the radix tree, flattening of the index into arrays, compilation of routes and policies, merging of header operations, and header predicates |
| `internal/controlplane/loader` | 1 | — | loading of the manifest from a file and the joint operation of the decoder and validation |
| `internal/dataplane/router` | 4 | 7 | path matching, header predicates, and lookup cost on deep and wide trees |
| `internal/dataplane/proxy` | 4 | 8 | `Executor`: routing and canonical paths, status codes on upstream failure, and network cost (Section 7) |
| `internal/dataplane/retry` | 5 | — | retries: idempotency, request body, attempt timeout, and cancellation of the request context |
| `internal/dataplane/request` | 6 | 1 | canonicalization of paths, construction of upstream headers and `X-Forwarded-*` |
| `internal/config` | 5 | — | loading on top of `DefaultRuntime`: a minimal document yields the defaults, explicit values (including a zero) override them, the `middleware` switches, rejection of an unknown key, and the required listen addresses |
| `internal/edge/public` | 2 | — | composition of the public middleware chain for each combination of the `middleware` switches |
| `internal/dataplane/middleware` | 5 | 1 | `X-Request-Id`: acceptance of a valid value, replacement of an invalid one, and absence of allocations |
| `internal/httpmethod` | 2 | — | parsing of methods and bitmasks |

Packages without unit tests are listed in Section 9.

### 4.1 Conventions for unit tests

- Checks are organized as table-driven tests: a `map` of named cases iterated with `t.Run` (see [request_id_test.go](../internal/dataplane/middleware/request_id_test.go)).
- Helper functions call `t.Helper()` so that failures are reported at the call site.
- The absence of allocations is asserted with `testing.AllocsPerRun`; for example, `TestRequestIDDoesNotAllocateForValidClientID`. Such a test fails when an allocation appears on the hot path, even if the functional behavior is unchanged.

---

## 5. Integration tests (`tests/integration`)

The integration tests construct a routing engine from a manually specified `snapshot.CompiledConfig`, not from YAML. They verify the interaction of `router.Engine`, `httpmethod`, and `policy` through their exported interfaces.

**Table 4.** Integration tests.

| File | Test | Verification |
|---|---|---|
| [proxy_test.go](../tests/integration/proxy_test.go) | `TestRouting_MatchesPathMethodAndHeaders` | a route is selected when the path, method, and headers all match |
| | `TestRouting_UnknownPathIsNotFound` | an unknown path matches no route |
| | `TestRouting_WrongMethodIsMethodNotAllowed` | the path matches but the method does not; the result is `405` |
| | `TestRouting_MissingRequiredHeaderIsNotFound` | the absence of a required header yields `404`, not `405` ([routing-and-errors.md](routing-and-errors.md), Section 2.1) |
| [policies_test.go](../tests/integration/policies_test.go) | `TestExecuteMutations_OrderingAndScopes` | `remove` is applied before `add` (add-if-absent); an existing header is not overwritten |
| | `TestExecuteMutations_NilPlanIsNoop` | the absence of a plan leaves the headers unchanged |
| | `TestRateLimiterSet_EnforcesBurstThenRejects` | requests are rejected once `burst` is exhausted |
| | `TestRateLimiterSet_NoRateLimitAlwaysAllows` | a route without a rate-limit policy is never rejected |

---

## 6. End-to-end tests (`tests/e2e`)

The end-to-end tests traverse the path that the process follows at startup, excluding flag parsing, signal handling, and the opening of real sockets. The function `bootstrapTestGateway` ([e2e_test.go](../tests/e2e/e2e_test.go)) performs the following steps:

1. it creates `aegis.yaml` and `gateway.yaml` in a temporary directory;
2. it loads both files with `config.Load` and `loader.Load`, the same functions that `app.New` uses;
3. it invokes `app.Bootstrap` and obtains the ready HTTP handlers;
4. it serves the handlers through `httptest.NewServer` and sends requests with the `net/http` client.

The upstream is represented by an `httptest` server that records the headers it receives. This permits request-side policies to be verified against what the upstream actually received, rather than inferred from the gateway's response.

**Table 5.** End-to-end scenarios.

| Test | Scenario | Expected result |
|---|---|---|
| `TestGateway_ProxiesMatchedRouteAndAppliesHeaderPolicy` | `GET /api/v1/profile` with an `X-Forwarded-For` header | `200`, the response body, and `nosniff` and `DENY` in the response |
| `TestGateway_RequestSidePolicyReachesUpstream` | the same request, with headers verified at the upstream | the client-supplied `X-Forwarded-For` does not reach the upstream |
| `TestGateway_ForwardsAndEchoesRequestID` | a client-supplied `X-Request-ID` | the value is forwarded upstream and returned to the client |
| `TestGateway_MethodNotAllowed` | a method not permitted by the route | `405` |
| `TestGateway_UnknownRouteReturnsNotFound` | a path outside the route table | `404` |
| `TestGateway_SystemListenerReportsLiveness` | `GET /livez` on the system handler | `200` |
| `TestGateway_ServiceTimeoutReturnsGatewayTimeout` | `GET /api/v1/profile/slow`; the upstream does not respond; timeout `200ms` | `504` |

### 6.1 Global metrics registry

`app.Bootstrap` registers its metrics with `prometheus.DefaultRegisterer`. Registering the same names a second time causes a panic. For this reason, `bootstrapTestGateway` replaces the registry before each invocation:

```go
prometheus.DefaultRegisterer = prometheus.NewRegistry()
```

Any test added to this package MUST perform the same substitution. Omitting it causes a panic that occurs only on the second invocation of `Bootstrap` within a single process.

---

## 7. Benchmarks

Benchmarks measure the cost of the hot path. They are not executed by `go test` by default, because `go test` runs only `Test*` functions. Benchmarks are executed with the `-bench` flag.

**Table 6.** Benchmarks by package.

| Package | Benchmarks | Measurement |
|---|---|---|
| `internal/dataplane/router` | `BenchmarkLookupHighFanout`, `BenchmarkLookupDeep`, `BenchmarkLookupMixed`, `BenchmarkMethodAdmission`, `BenchmarkHeadersMatch`, `BenchmarkRequestMatch`, `BenchmarkRequestMatchParallel` | lookup in trees of varying depth and breadth; method and header checks |
| `internal/dataplane/proxy` | `BenchmarkProxyServeHTTP` and variants (`EmptyHeaders`, `RealisticHeaders`, `WithHeaderPolicy`, `Parallel`, `RealTransport`) | cost of `Executor`, without and with network I/O |
| `internal/dataplane/proxy` | `BenchmarkRoundTripWarm`, `BenchmarkFullProxyE2E` | cost of the transport and of the complete path over the network |
| `internal/dataplane/request` | `BenchmarkUpstreamHeader` | construction of upstream request headers |
| `internal/dataplane/middleware` | `BenchmarkNewRequestID` | generation of UUIDv7 identifiers |

### 7.1 Execution

The router benchmarks are executed by the `make bench` target:

```bash
go test ./internal/dataplane/router/ -bench . -benchmem
```

The proxy benchmarks are organized in three layers, which together attribute latency to its sources. The rationale is documented in [executor_network_bench_test.go](../internal/dataplane/proxy/executor_network_bench_test.go):

```bash
go test ./internal/dataplane/proxy/ -run '^$' \
    -bench 'BenchmarkProxyServeHTTP$|BenchmarkRoundTripWarm|BenchmarkFullProxyE2E' \
    -benchmem -count 10
```

The flag `-run '^$'` suppresses ordinary tests in that invocation. The flag `-count 10` specifies the number of repetitions. To compare results before and after a change, `benchstat` (`golang.org/x/perf/cmd/benchstat`) is recommended.

Benchmark results depend on the machine. Only measurements taken on the same machine under comparable conditions are meaningful when compared.

### 7.2 Comparison with other gateways

The directory `bench/` contains a comparative setup for Aegis, Traefik, and a direct connection to the upstream. The setup is executed by `bench/compare.py`, which uses `wrk` and `vegeta`. It is not part of `go test` and is not executed in continuous integration. Its results are stored in `bench/results/`. The methodology, including the rules for a fair comparison, is described at the beginning of the script.

---

## 8. Static analysis and continuous integration

### 8.1 Linter

The configuration [.golangci.yml](../.golangci.yml) enables the standard set of linters (`errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused`) and the `gofmt` formatter. The `govet` linter subsumes `go vet`, so a separate vet step in CI is not required.

The version of the linter is pinned in CI to `golangci-lint v2.14.0` ([workflow.yml](../.github/workflows/workflow.yml)). At the time of measurement, version 2.13.1 was installed locally. Results may differ between versions; local verification SHOULD use the version pinned in CI.

### 8.2 Pipeline

The CI pipeline ([workflow.yml](../.github/workflows/workflow.yml)) runs on pushes to `main` and on every pull request. It consists of two jobs:

| Job | Steps |
|---|---|
| `test` | `go mod download`, `go test ./... -v`, `go test ./... -race` |
| `lint` | `golangci-lint` version `v2.14.0` |

The Go version in CI is `1.27.x`. Concurrent runs for the same reference cancel one another.

Benchmarks are not executed in CI, and the comparative setup is not executed either.

---

## 9. Coverage

### 9.1 Method

The command `go test ./... -cover` reports coverage only for the package that owns the test. For packages without tests of their own, it reports 0%, even when they are fully exercised by the end-to-end tests. For this reason, the assessment below was obtained with cross-package coverage:

```bash
go test ./internal/... ./tests/... -coverpkg=./internal/... -count=1 -coverprofile=cover.out
```

The values were obtained by summing the blocks of the coverage profile per directory. They are approximate and reflect the state on 2026-10-14.

**Table 7.** Statement coverage by package (cross-package).

| Package | Coverage | Remark |
|---|---|---|
| `internal/controlplane/loader` | 100.0 % | |
| `internal/dataplane/request` | 100.0 % | |
| `internal/dataplane/router` | 100.0 % | |
| `internal/httpmethod` | 100.0 % | |
| `internal/observe/metrics` | 100.0 % | |
| `internal/snapshot` | 98.4 % | |
| `internal/controlplane/validate` | 98.0 % | |
| `internal/dataplane/proxy` | 97.6 % | |
| `internal/edge/public` | 98.5 % | |
| `internal/controlplane/compile` | 94.5 % | |
| `internal/dataplane/middleware` | 95.3 % | |
| `internal/controlplane/pipeline` | 90.7 % | |
| `internal/dataplane/retry` | 82.6 % | |
| `internal/controlplane/normalize` | 70.9 % | exercised only through the end-to-end tests |
| `internal/dataplane/policy` | 70.7 % | |
| `internal/edge/admin` | 67.6 % | |
| `internal/config` | 53.8 % | no tests of its own (Section 9.2) |
| `internal/observe/health` | 33.3 % | |
| `internal/dataplane/routelabel` | 40.0 % | |
| `internal/app` | 28.2 % | |

The aggregate coverage across `internal/...` is approximately 85 %. This figure conceals an uneven distribution; the per-package values in Table 7 SHOULD be used in any assessment.

### 9.2 Gaps

The following areas are insufficiently tested to support confident modification. They are listed in order of decreasing risk.

1. **`internal/dataplane/routelabel` and the metrics integration.** The package `routelabel` is invoked from `Executor`, but no context carrying a recorder is ever created (`routelabel.NewContext` has no caller), and the metrics layer does not read the value. Route-based labeling is therefore not operational. No test detects this, because the end-to-end tests do not inspect metric labels. A test that reads `/metrics` and asserts the route label would expose the defect.
2. **`internal/config` has tests only for loading, defaults and the `middleware` section.** The rules of `Validate`, `ResolvePath`, and `BuildTLSConfig` are verified only indirectly. A table-driven test for each rule in [runtime-configuration.md](runtime-configuration.md), Section 7, is recommended.
3. **`internal/app`, lifecycle.** Coverage is 28 %. The shutdown sequence, the propagation of a listener failure to the whole process, and the transition of `/readyz` to the not-ready state during shutdown are not tested. A test of `Lifecycle.Run` that cancels the context and verifies termination of both listeners is recommended.
4. **`internal/observe/health` and `/readyz`.** Only `/livez` is tested (Section 6). The readiness state during shutdown is verified only manually.
5. **TLS.** No test starts an HTTPS listener. Certificate loading is verified only during manual checks.
6. **Components without direct tests.** `request.RemoveHopHeaders` (in `hopbyhop.go`), `middleware.TimeoutMiddleware`, and `middleware.MetricsMiddleware` have neither unit tests nor end-to-end tests that address them directly. The timeout behavior is exercised indirectly by `TestGateway_ServiceTimeoutReturnsGatewayTimeout`, but the expiry of `request_timeout` and the recording of metrics by status code are not verified.

---

## 10. Manual verification

Manual verification is useful for confirming behavior under a configuration close to a deployment. It does not replace the automated tests.

### 10.1 Docker Compose

```bash
make up
```

The command builds the image and starts two containers: `aegis` and `mock-upstream`. The public listener is available at `localhost:8080`; the system listener at `localhost:18080`.

Verification of the `user-profile` route. The route requires the four headers listed in [gateway.yaml](../configs/gateway.yaml). Without them the response is `404` ([routing-and-errors.md](routing-and-errors.md), Section 2.1):

```bash
curl -i http://localhost:8080/api/v1/profile \
  -H 'Authorization: Bearer test-token' \
  -H 'X-Client-Channel: web' \
  -H 'X-Device-Id: device-1' \
  -H 'X-Correlation-Id: corr-1'
```

The expected result is status `200`, the body `{"status":"ok"}`, the headers `X-Content-Type-Options: nosniff` and `X-Frame-Options: DENY`, and an echo of `X-Request-Id`. Requests sustained above 100 per second receive `429`, because the policy `profile-rate-limit` specifies `rate: 100` and `burst: 1`.

Verification of the system listener:

```bash
curl -i http://localhost:18080/livez
curl -i http://localhost:18080/readyz
curl -s http://localhost:18080/metrics | grep '^aegis_'
```

### 10.2 Execution from source

The target `make run` starts the process with `configs/aegis.yaml` and `configs/gateway.yaml`. The routing manifest refers to the host `mock-upstream`, which exists only within Docker Compose. Without it, every request to `user-profile` and `benchmark` terminates with `502`. Outside Compose, the host name SHOULD be mapped to a local address, for example by adding `127.0.0.1 mock-upstream` to `/etc/hosts`, and the stand-in SHOULD be started:

```bash
MOCK_ADDR=:8082 go run ./mock
```

The stand-in listens on a single port. The service `order-service` refers to port 8083, on which the stand-in does not respond; in this mode the route `user-orders` returns `502`.

---

## 11. Conventions for test code

- **Names.** Tests are named `Test<Subject>_<Behavior>`, for example `TestRouting_UnknownPathIsNotFound` or `TestGateway_ServiceTimeoutReturnsGatewayTimeout`. Table cases are named by a string describing the condition, for example `"too long"` or `"repeated"`.
- **Helpers.** Helper functions call `t.Helper()`. Temporary files are created with `t.TempDir()`, and resources are released with `t.Cleanup`.
- **Network tests.** These use `httptest.NewServer` and addresses of the form `:0`, so that they do not conflict with ports in use.
- **Assertions.** Tests compare status codes and response bodies, not the text of internal errors.
- **Side effects.** Global state, such as the metrics registry, is replaced for the duration of a test (Section 6.1).

---

## 12. Recommendations

The recommendations are listed in decreasing order of expected value:

1. Add a test for route-based metric labels (Section 9.2, item 1). It will show whether `routelabel` is connected to the metrics.
2. Add table-driven tests for `internal/config` (Section 9.2, item 2).
3. Add tests for the lifecycle and for readiness (Section 9.2, items 3 and 4).
4. Add an end-to-end test of the HTTPS listener using a self-signed certificate created in a temporary directory.
5. Consider executing the proxy benchmarks in CI with recorded results, so that regressions in cost are detected when code changes. This requires a stable machine; results on shared runners are expected to be noisy.
