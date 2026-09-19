# Aegis Gateway Configuration

This document specifies the configuration model, field semantics, validation
rules, and request-processing semantics of Aegis.

The configuration is declarative. It describes the desired gateway behavior;
it does not describe the internal execution model of the gateway.

Aegis loads the configuration, normalizes and validates its contents, and
compiles it into an immutable runtime representation. The data plane operates
on this compiled representation and does not interpret YAML configuration
during request processing.

## Overall structure

```yaml
services:
  <service-name>:
    upstream: ...

routes:
  - name: <route-name>
    service: <service-name>
    match: ...
    policies: ...

policies:
  headers:
    <policy-name>: ...
  rate_limits:
    <policy-name>: ...
```

Routes reference services by name, and route policies reference entries from the `policies` section by name. The order of sections in the file does not matter, but names within each section must be unique.

## The `services` section

The `services` section describes the backend services to which the gateway can proxy requests. Each service is defined as a name-to-upstream pair.

```yaml
services:
  user-profile:
    upstream:
      scheme: http
      host: mock-upstream
      port: 8082
```

The `upstream` field consists of three required fields:

- **scheme** — the connection scheme for the upstream service (`http` or `https`);
- **host** — the network name or address of the upstream service;
- **port** — the port on which the upstream service accepts connections.

The service name (the key of the `services` map) is used solely as a reference within the configuration — in the `service` field of a route entry. It carries no other meaning.

## The `routes` section

The `routes` section is an ordered list of routing rules. Each rule ties a matching condition (`match`) to a target service and a set of policies.

```yaml
routes:
  - name: user-profile
    service: user-profile
    match:
      path_prefix: /api/v1/profile
      methods: [GET]
      headers:
        Authorization: []
        X-Client-Channel: [web, mobile]
    policies:
      - name: security-headers
      - name: profile-rate-limit
```

### The `name` field

The route name is a stable identifier used in diagnostics, metrics, and logs. This field is required: an empty name signals a configuration compilation error, not a valid state. It is exactly this `name` that ends up in metric labels distinguishing routes after a request has been successfully matched.

### The `service` field

A reference to a service from the `services` section, by name. When a request is matched to the route, the service named here becomes the upstream destination of the proxied request.

### The `match` field

The `match` field holds the conditions under which a route applies to a request. It consists of three parts: `path_prefix`, `methods`, and `headers`.

#### `path_prefix`

`path_prefix` specifies the path against which the request path is matched. Matching is built on segment boundaries rather than raw string prefixes, so a route with `path_prefix: /api/v1/profile` matches both that path itself and any path that continues further along its segments:

```
path_prefix: /api/v1/profile

/api/v1/profile              — matches
/api/v1/profile/123          — matches
/api/v1/profile/123/settings — matches
/api/v1/profiles             — does not match
```

The last example illustrates the main consequence of segment-based matching: a shorter path never "swallows" a neighboring path that merely shares its leading characters without sharing a segment boundary.

If a request matches several routes with `path_prefix` values of different lengths, the more specific one wins — that is, the one whose prefix is longer and describes the request path more precisely.

Besides static segments, a route's prefix may contain parameterized and wildcard segments:

- `:name` — matches exactly one path segment and binds its value to the parameter name;
- `*name` — matches the remainder of the path, however many segments deep.

Both kinds of segments are subject to the same prefix-matching principle as static paths: a route registered at a parameter or wildcard node serves as a fallback answer for any path that continues past that node, once nothing more specific has been found deeper in the tree.

When several candidates are available, priority is always resolved in the following order, from most to least specific, and applied recursively at every path segment:

1. static segment;
2. parameter segment (`:name`);
3. wildcard segment (`*name`).

It is important to understand that this priority concerns path structure alone and does not take the request's method or headers into account — those are checked separately, after structural matching. If the most specific structural match does not support the request's method or headers, the gateway does not give up the search; it moves on to the next candidate in priority order — exactly the way a person reading a route table would expect a fallback route to behave.

#### `methods`

`methods` is a list of HTTP methods allowed for the route:

```yaml
methods: [GET, POST]
```

If the list is empty or omitted, the route accepts requests with any method, provided the path and headers match. Specifying particular methods restricts the route's applicability to those alone.

#### `headers`

`headers` is a map of headers that must be present in the request for the route to be considered applicable. The key is the header name; the value is a list of acceptable values:

```yaml
headers:
  Authorization: []
  X-Client-Channel: [web, mobile]
```

The semantics of the value depend on whether the list is empty:

- an empty list means the header must be present, and its specific value does not matter;
- a non-empty list means the header must be present, and at least one of the request's values must match one of the listed values.

Header predicates are intentionally limited to equality checks and do not support regular expressions or logical `AND`/`OR` expressions — this is a deliberate simplification that keeps header checking at request time predictable and fast.

If a request's path and method match a route but its headers do not, that route is effectively invisible to the client that sent this particular request — not merely in a usability sense, but literally: the server responds as if no such path existed at all, rather than as if the path existed but required a different method.

### The `policies` field

A list of references to policies applied to the route, each given by name:

```yaml
policies:
  - name: security-headers
  - name: profile-rate-limit
```

The order of entries in this list does not determine the order of application — the application order is fixed separately for each kind of policy and is described below. A route may reference any number of policies, including none at all.

## The `policies` section

The `policies` section contains reusable policy definitions referenced by routes. It consists of two maps: `headers` and `rate_limits`.

### `headers`

Each entry describes a set of operations on request and response headers:

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

The `request` section is applied to the request before it is sent to the upstream service; the `response` section is applied to the upstream service's response before it is returned to the client. Each of them consists of three operations:

- **add** — sets the header only if it is not already present;
- **set** — unconditionally overwrites the header's value;
- **remove** — deletes the listed headers, if present.

The order in which operations are applied is fixed and does not depend on the order in which they are written in the file: removal is performed first, then unconditional setting, then conditional addition. This guarantees a predictable result regardless of the order in which the configuration author listed the operations.

The same header must not appear in more than one operation group for the same direction at once — such a configuration is considered invalid and should be rejected during validation, before compilation.

One point deserves particular attention: the ordering between the request-header policy and the forwarding headers the gateway generates on its own (for example, `X-Forwarded-For`). The policy is applied after the gateway has already assembled the complete set of its own forwarding headers, so a rule such as `remove: X-Forwarded-For` removes both a client-supplied value and any value generated by the gateway itself.

### `rate_limits`

Each entry describes a rate-limiting policy:

```yaml
policies:
  rate_limits:
    profile-rate-limit:
      rate: 100
      burst: 1
```

- **rate** — the sustained refill rate of the limit, i.e. the allowed number of requests per second over the long run;
- **burst** — the maximum size of a short-term spike of requests above the sustained rate.

A route with no rate-limiting policy attached is served without any restriction — this is a deliberately chosen default behavior, not a side effect of missing configuration.

## Complete configuration example

```yaml
services:
  user-profile:
    upstream:
      scheme: http
      host: mock-upstream
      port: 8082

routes:
  - name: user-profile
    service: user-profile
    match:
      path_prefix: /api/v1/profile
      methods: [GET]
      headers:
        Authorization: []
        X-Client-Channel: [web, mobile]
    policies:
      - name: security-headers
      - name: profile-rate-limit

policies:
  headers:
    security-headers:
      request:
        add:
          X-Request-Id: "<generated-or-from-incoming>"
        remove:
          - X-Forwarded-For
      response:
        add:
          X-Content-Type-Options: "nosniff"
        remove:
          - Server

  rate_limits:
    profile-rate-limit:
      rate: 100
      burst: 1
```

This configuration describes a single service, `user-profile`; a single route to it, restricted to the `GET` method and requiring certain headers; and two policies — one governing request and response headers, and one limiting the rate of requests to the route.