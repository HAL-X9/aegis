# Policies Specification

This document defines the control-plane policy model for header mutations.
Policies are declarative, reusable objects referenced by routes.

## Scope

- Supported policy family: `headers`
- Not in scope: `cors` (reserved for future work)

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

- `request`: applied to outbound request headers before proxying to upstream.
- `response`: applied to outbound response headers before sending to client.

Each group supports three operations:

- `add`: add `<name>: <value>` only if `<name>` does not already exist.
- `set`: set `<name>: <value>` unconditionally (overwrite if present).
- `remove`: remove every header listed by name.

### Operation Ordering

Within one operation group, processing order is:

1. `remove`
2. `set`
3. `add`

This order ensures deterministic behavior and prevents accidental reintroduction
of removed headers by `add`.

### Validation Requirements

The control plane must reject invalid policies before runtime. Minimum checks:

- Policy names under `policies.headers` are non-empty and unique.
- Header names in `add`, `set`, and `remove` are non-empty.
- Header names are compared case-insensitively for conflict detection.
- A header name must not appear in more than one operation group (`add`, `set`,
  `remove`) within the same direction (`request` or `response`).

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

## Notes

- Security response headers (for example, `X-Frame-Options`) belong in
  `response`, not `request`.
- `request` mutations should be limited to trusted forwarding context,
  correlation, and transport metadata required by upstream services.
