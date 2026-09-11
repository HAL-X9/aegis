# Policies Specification

This document defines the control-plane policy model for header mutations
and its runtime execution. Policies are declarative, reusable objects
referenced by routes.

## Scope

- Supported policy family: `headers`
- Not in scope: `cors` (reserved for future work)

## Implementation Status (Current Codebase)

Headers policy compilation, conflict validation, and runtime mutation
application are implemented:

- `internal/controlplane/compile/routes_compiler.go` merges every
  headers-policy a route references into a single set of operations
  (`mergeHeadersOps`), rejecting a route whose referenced policies assign
  conflicting operations to the same header name
  (`headersOpContains` / "header %q has conflicting operations").
- `internal/controlplane/compile/policies_compiler.go` compiles each
  named headers policy into `snapshot.CompiledHeaders`: a
  `snapshot.HeaderInstruction` per operation, carrying a well-known
  `HeaderID` and — for `set`/`add` — the literal value, computed once at
  compile time.
- `internal/dataplane/policy/headers.go` (`ExecuteMutations`) applies a
  compiled plan to an `http.Header` at request time: `remove`, then `set`,
  then `add` (add-if-absent), in that fixed order.

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
it was compiled, without re-sorting). It guarantees deterministic
behavior and prevents `add` from reintroducing a header the same policy
just removed.

### Validation Requirements

The control plane rejects invalid policies before runtime:

- Header names in `add`, `set`, and `remove` must resolve to a known
  `HeaderID` (`compile.resolveHeaderID`); an unresolvable name fails
  compilation with an error naming the offending policy and header.
- When a route references more than one headers policy, the merged
  operation set must not assign conflicting operations
  (`add`/`set`/`remove`) to the same header name within the same
  direction (`request` or `response`); a conflict fails route compilation.
- A route referencing an undefined policy name (neither a headers policy
  nor a rate-limit policy) fails compilation
  (`validatePolicyRefsExist`).

Header name matching for conflict detection is performed against the
canonical `HeaderID` each name resolves to, which is equivalent to
case-insensitive comparison for all currently supported header names.

## Example

```yaml
policies:
  headers:
    security-headers:
      request:
        add:
          X-Request-Id: "<generated-or-from-incoming>"
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
applied on both the request and response path. `RemoveHopHeaders` runs
after header-policy mutations on the request side and after response
headers are copied from upstream on the response side; a headers policy
must not be relied upon to strip connection-scoped headers
(`Connection`, `Transfer-Encoding`, `Keep-Alive`, and related headers) —
that is `RemoveHopHeaders`'s responsibility regardless of policy
configuration.

## Notes

- Security response headers (for example, `X-Frame-Options`) belong in
  `response`, not `request`.
- `request` mutations should be limited to trusted forwarding context,
  correlation, and transport metadata required by upstream services.