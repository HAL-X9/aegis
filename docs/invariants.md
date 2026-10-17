# Security Invariants

This document is the normative source for the invariants identified as
`I-001` ... `I-010`. Code comments at the enforcing sites cite these
identifiers. Where a comment and this file disagree, this file wins.

An invariant with no enforcing code listed is specified but not yet
implemented. Each entry records four things: the threat it addresses,
the property it guarantees, where that property is enforced, and how
the enforcement is verified. An invariant for which these cannot be
answered does not belong here.

Verification levels used below:

- **Test**: unit or integration test in the repository.
- **Fuzz**: `go test -fuzz` with an invariant oracle.
- **Gobra**: deductive proof of a function (planned).
- **Quint**: bounded model checking with Apalache (planned).
- **Diff**: differential run against Traefik, Envoy or NGINX.

---

## Scope

**Boundary.** External client -> Gateway (`../internal/dataplane`) ->
upstream service.

**Asset.** The resource a request is authorized for must be the
resource the upstream serves.

**Adversary.** An external client who controls the request line,
method, headers and body. Any byte sequence is a valid input.

**Trusted base.**

1. Route and policy configuration is correct and validated by
   `../internal/controlplane`.
2. The Go standard library behaves as documented.
3. Every supported upstream treats `/` as the only path separator.
   This holds only for paths accepted under I-004 and I-005.

Assumption 3 is not proven by Aegis. If an upstream violates it, the
guarantees of I-001 to I-005 do not extend to that upstream.

---

## Path Canonicalization

### I-001 Routed path equals forwarded path

**Threat.** A request is authorized by one route and served from a
different resource, because the router and the upstream read the path
differently.

**Property.** For every accepted request, the path used to select the
route is the path the upstream receives.

**Enforcement.** Implemented. `proxy/executor.go` builds the upstream
request from the canonical path. `RawPath` is copied only when the
path is unchanged.

**Verification.** Test (`executor_test.go`). Fuzz (`FuzzExecutorPathAgreement`):
the service that receives a request is the one the first segment of the
forwarded path selects. Gobra planned.

### I-002 Canonical path has no dot segments

**Threat.** Traversal across a route boundary, for example
`/public/../admin`.

**Property.** No segment of the canonical path is `.` or `..`.

**Enforcement.** Implemented. `request/path.go`, `CanonicalPath`,
using `path.Clean` and explicit handling of a trailing `/.` and `/..`.

**Verification.** Test (`path_test.go`). Fuzz (`FuzzCanonicalPath`). Gobra
planned.

### I-003 Canonicalization is idempotent

**Threat.** A path that changes again on a second pass, so that
routing and forwarding see different values.

**Property.** `CanonicalPath(CanonicalPath(p)) == CanonicalPath(p)`.

**Enforcement.** Implemented. `isCanonical` returns the input
unchanged when it is already canonical.

**Verification.** Test. Fuzz (`FuzzCanonicalPath`). Gobra planned.

### I-004 No ambiguous separators in the canonical path

**Threat.** The gateway and an upstream split a path differently.
Several common upstreams (IIS, Tomcat, frameworks built on them)
treat `\` or `;` as separators. Gateway routing under `/public` then
delivers `/admin` on the upstream. Related CVEs: Traefik
CVE-2025-66490, Envoy CVE-2026-73551.

**Property.** The canonical path contains no `\`, `;` or NUL byte.
A request whose decoded or raw path contains `\`, `;`, NUL, `%5C`,
`%3B` or `%00` is answered `400 Bad Request`.

**Enforcement.** **Not implemented.** These characters currently pass
through `CanonicalPath`. Example: `/public/..\admin/x` is routed under
`/public`.

**Verification.** Planned: Test on each rejected character and
boundary case (`/a%5Cb`, `/a%3Bb`, `/a%00b`), Fuzz with this property
as oracle, Gobra on the rejection predicate, Diff against upstreams
that treat `\` as a separator.

### I-005 Encoded slashes are rejected

**Threat.** The router splits on `%2F` and the upstream does not, or
the reverse. Related CVE: Envoy CVE-2021-29492, which concerned both
`%2F` and `%5C`.

**Property.** A raw path containing `%2F` or `%5C` (any case) is
answered `400 Bad Request`.

**Enforcement.** **Partial.** `hasEncodedSlash` rejects `%2F` and
`%2f`. `%5C` is not rejected. This is inconsistent with the threat
model and is closed by the fix for I-004.

**Verification.** Test for `%2F` (`path_test.go`). Test for `%5C`
missing.

---

## Routing

### I-006 Selected route's predicates hold

**Threat.** A request is forwarded to a route whose method or header
predicates it does not satisfy.

**Property.** The selected route allows the request's method, and
every header predicate of the route is satisfied by the request.

**Enforcement.** Implemented. The method bit check and
`router.HeadersMatch` run before the route is returned from the
candidate loop in `proxy/executor.go`.

**Verification.** Test (`engine_test.go`, `match_headers_test.go`).
Quint planned.

### I-007 No shadowing by a higher-priority route

**Threat.** A request that matches a high-priority route is silently
served by a lower-priority route, because a predicate of the first
route failed. The lower route may be permissive.

**Property.** A lower-priority route is selected only if every
higher-priority candidate that matches the path fails on method or
header predicates. Both failures are recorded and produce the correct
status (404 or 405), as specified in `routing-and-errors.md`.

**Enforcement.** Implemented. The loop in `proxy/executor.go` and
`router.Engine.Lookup` continue past a failed predicate.

**Verification.** Code reading and tests. Quint with Apalache planned
as the primary verification.

---

## Headers

### I-008 Header policy operations take effect upstream

**Threat.** A policy the operator relies on (for example, a `set` of a
security header) is silently not applied, so the upstream receives
different headers than the configuration states.

**Property.** Each `set`, `add` and `remove` operation of a route's
request policy is reflected in the request sent upstream. For `Host`,
the value is applied through `req.Host`, because `net/http` ignores
`Header["Host"]` when writing a request.

**Enforcement.** **Not implemented for `Host`.** `policy/headers.go`
writes `Header["Host"]`, which the transport ignores. Other operations
are implemented.

**Verification.** Planned: Test that `set Host` changes the upstream
`Host`. No test file exists yet under `dataplane/policy/`.

### I-009 Hop-by-hop headers do not cross either hop

**Threat.** Connection-scoped headers, including those named in
`Connection`, are forwarded and change connection semantics on the
upstream or the client side.

**Property.** `Connection`, `Keep-Alive`, `Proxy-Authenticate`,
`Proxy-Authorization`, `Proxy-Connection`, `TE`, `Trailer`,
`Transfer-Encoding` and `Upgrade`, and any header named in
`Connection`, are removed from the request sent upstream and from the
response sent to the client.

**Enforcement.** Implemented. `request/hopbyhop.go`, called after
policy on the request and after policy on the response in
`proxy/executor.go`.

**Verification.** Planned: a direct test of `RemoveHopHeaders` with a
`Connection`-named header. The existing `header_test.go` covers
`UpstreamHeader`, not this function.

### I-010 Client-supplied forwarding identity is not trusted

**Threat.** The client sets headers that the upstream treats as the
true path, origin or client address. Examples: `X-Original-URL`,
`X-Rewrite-URL`, `X-Forwarded-Prefix`, `Forwarded`, `X-Real-IP`.
Frameworks and IIS-style stacks honor some of these and can override
routing decisions made by the gateway.

**Property.** Headers that carry the path, origin or client address
as seen by the gateway are either removed or replaced with values the
gateway derived, before the request is sent upstream.

**Enforcement.** **Not implemented by default.** `X-Forwarded-For`,
`X-Forwarded-Proto` and `X-Forwarded-Host` are replaced
(`request/header.go`). The other headers pass through unchanged unless
a route policy removes them explicitly.

**Verification.** Planned: Test that the listed headers are absent
or replaced upstream. Diff against an upstream that honors
`X-Original-URL`.

---

## Summary

| ID    | Property                        | Status              | Verification        |
|-------|---------------------------------|---------------------|---------------------|
| I-001 | Routed path = forwarded path    | Implemented         | Test; Fuzz; Gobra planned |
| I-002 | No dot segments                 | Implemented         | Test; Fuzz; Gobra planned |
| I-003 | Idempotent canonicalization     | Implemented         | Test; Fuzz; Gobra planned |
| I-004 | No ambiguous separators         | **Not implemented** | Planned             |
| I-005 | Encoded slashes rejected        | Partial             | Test; `%5C` missing |
| I-006 | Selected route predicates hold  | Implemented         | Test; Quint planned |
| I-007 | No shadowing                    | Implemented         | Test; Quint planned |
| I-008 | Header policy takes effect      | Partial (`Host`)    | Planned             |
| I-009 | Hop-by-hop headers stripped     | Implemented         | Test                |
| I-010 | Forwarding identity not trusted | **Not implemented** | Planned             |

Six invariants are implemented. Two are partial. Two are not
implemented. The table must be updated when an invariant gains its
enforcing code.

---

## Known Gaps and Assumptions

Listed here so that they are recognized as decisions, not oversights.

- **Route predicates on headers are client-controlled.** A route
  selected by a header such as `X-Role: admin` is selected for any
  client that sends that header. Header predicates are a routing
  feature, not an authorization mechanism. Routes that protect
  privileged upstreams must not rely on them.
- **Semicolons in legitimate paths.** Rejecting `;` under I-004 breaks
  paths that use matrix parameters. The trade-off is open.
- **Request smuggling (CL/TE).** Behavior of the Go server and
  transport when both `Content-Length` and `Transfer-Encoding` are
  present has not been verified. Related: Kong CVE-2026-6338, not yet
  confirmed from a primary source. No invariant covers this yet.
- **Upstream list.** Assumption 3 must be stated per supported
  upstream. The project does not yet maintain that list.
- **Bounded verification.** Quint and Apalache results hold up to a
  bounded path length and alphabet. Gobra results hold for the
  canonicalization function under the stated trusted base, not for the
  proxy as a whole.
- **Authorization.** This document covers path and routing
  invariants. What an authenticated principal may do upstream is out
  of scope.

---

## Related Documents

- [Routing semantics and error responses](routing-and-errors.md)
