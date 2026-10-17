# Policies Specification

This document defines the control-plane policy model for header mutations
and its runtime execution. Policies are declarative, reusable objects
referenced by routes.

## Scope

- Supported policy family: `headers`
- Not in scope: `cors` (reserved for future work)

## Implementation Status (Current Codebase)

Headers policy validation, merging, and runtime mutation are implemented:

- `internal/controlplane/validate` rejects conflicting operations within one
  policy and across the headers policies a route references (see Validation
  Requirements).
- `internal/controlplane/compile/routes_compiler.go` merges the headers
  policies a route references into one set of operations (`mergeHeadersOps`)
  and compiles the result into that route's `CompiledRoutePolicies.Headers`.
  Headers policies are not kept as separate compiled objects.
- `internal/controlplane/compile/headers_registry.go` assigns every header name
  a `snapshot.HeaderID`: well-known names receive the fixed identifiers defined
  by the single table in `internal/snapshot/headers.go` (which the data plane
  also uses to resolve identifiers back to names), and other names receive
  dynamic identifiers that resolve through `snapshot.HeaderRegistry`. Each `snapshot.HeaderInstruction` carries its
  identifier and, for `set` and `add`, the literal value, computed once at
  compile time.
- `internal/dataplane/policy/headers.go` (`ExecuteMutations`) applies a compiled
  plan to an `http.Header` at request time: `remove`, then `set`, then `add`
  (add-if-absent), in that fixed order.

## Configuration Shape

Top-level policy configuration:

```yaml
policies:
  headers:
    <policy-name>:
      request:
        add: {}
        set: {}
        remove: []
      response:
        add: {}
        set: {}
        remove: []
```

Route-level reference:

```yaml
routes:
  - name: example
    # ... route match and upstream omitted
    policies:
      - name: security-headers
```

## Headers Policy Semantics

A headers policy contains two independent operation groups:

- `request`: applied to outbound request headers before proxying to
  upstream.
- `response`: applied to outbound response headers before sending to the
  client.

Each group supports three operations:

- `add`: add `<name>: <value>` only if `<name>` does not already exist
  on the header set at the time this operation runs.
- `set`: set `<name>: <value>` unconditionally (overwrite if present).
- `remove`: remove every header listed by name.

### Operation Ordering

Within one operation group, execution order is fixed:

1. `remove`
2. `set`
3. `add`

This order is enforced both at compile time (`compileHeaderOps` emits
instructions in this sequence) and at runtime
(`ExecuteMutations` executes the compiled instruction list in the order
it was compiled, without re-sorting). It makes the compiled plan
deterministic. Because validation forbids a header from belonging to more
than one operation group in a direction, the order does not change the
outcome of a validated configuration; it is retained as a safeguard for
any plan that bypasses validation.

### Validation Requirements

The control plane rejects invalid policies in the `validate` stage, before
compilation:

- Header names in `add`, `set`, and `remove` must be valid HTTP field names.
- Within one policy, a header may appear in only one operation group
  (`add`, `set`, or `remove`) per direction, and only once within that group.
- When a route references more than one headers policy, no header may be
  changed by two of them within the same direction (`request` or
  `response`). A policy referenced twice by the same route conflicts with
  itself.
- A route referencing an undefined policy name (neither a headers policy
  nor a rate-limit policy) is rejected. So is a route referencing more than
  one rate-limit policy.

Header names are compared case-insensitively when checking for conflicts.

## Example

```yaml
policies:
  headers:
    security-headers:
      request:
        remove:
          - X-Forwarded-For
          - X-Forwarded-Proto

      response:
        add:
          X-Content-Type-Options: "nosniff"
          X-Frame-Options: "DENY"
          X-XSS-Protection: "1; mode=block"
        remove:
          - Server
```

## Interaction with Hop-by-Hop Header Stripping

Header-policy execution and hop-by-hop header removal
(`internal/dataplane/request.RemoveHopHeaders`) are independent stages
applied on both the request path and the response path. On each path,
`RemoveHopHeaders` runs after the policy mutations: on the request side,
before the upstream call; on the response side, before the upstream
response headers are copied to the client. Consequently a headers policy
cannot cause a connection-scoped header to be forwarded, and must not be
relied upon to strip one (`Connection`, `Transfer-Encoding`, `Keep-Alive`,
and related headers); that is the responsibility of `RemoveHopHeaders`,
regardless of policy configuration.

The request-side `X-Forwarded-For`, `X-Forwarded-Proto`, and
`X-Forwarded-Host` headers are derived from the connection by
`request.UpstreamHeader` before the policy runs, so a request policy may
override or remove them.

## Limitations

- **`Host`.** A `set` or `add` of `Host` in a request policy writes
  `Header["Host"]` of the outbound request, which `net/http` ignores when
  sending it: the upstream receives the host of the service's `upstream`.
  The operation is accepted by validation and has no effect.

## Notes

- Security response headers (for example, `X-Frame-Options`) belong in
  `response`, not `request`.
- `request` mutations should be limited to trusted forwarding context,
  correlation, and transport metadata required by upstream services.