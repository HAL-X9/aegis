# Routing semantics and error responses

This document is the single source of truth for two things that must stay
in lock-step: how a request path is matched against the route table, and
which HTTP status code `proxy.Executor` returns when. Every status code below
is produced by `proxy.Executor.ServeHTTP` in `executor.go`.

## 1. Route matching semantics

### 1.0 The request path is canonicalized first

Before routing, `request.CanonicalPath` resolves dot segments (`.`, `..`)
per RFC 3986 §5.2.4 and keeps a trailing slash. It also merges empty
segments (`//`). RFC 3986 does not define this merge; it is an additional
normalization (see [canonical path specification](formal/canonical_path.md), D3).
The canonical path is the one both routed **and** forwarded
upstream, so a request can never match one route while addressing another
resource:

```
/public/../admin   -> routed and forwarded as /admin
/api//v1/./users   -> routed and forwarded as /api/v1/users
```

A path containing an encoded slash (`%2F`) has no unambiguous canonical
form — the router would split on it, an upstream may not — and is rejected
with `400`.

The specification further requires that a request be rejected with `400`
when its decoded path contains an octet outside the allowed set. The
allowed set is the RFC 3986 `pchar` set without `%` and `;`, with `/` as
the only separator. The rejected octets therefore include `\`, `;`, `%`,
`?`, `#`, NUL and control characters. Encoded forms that decode to them (`%5C`, `%3B`, `%25`, `%3F`,
`%23`, `%00`) are rejected in the same way. The full rule is in
[canonical path specification](formal/canonical_path.md), §3.

**Status.** The encoded-slash rejection and the dot-segment and empty-segment
handling are implemented. The rejection of the remaining octets listed above
is specified but **not yet enforced** by `CanonicalPath`: a request path
containing `\` or `;`, or one of the encoded forms, is currently routed and
forwarded.

Already-canonical paths are used as-is, without allocating.

### 1.1 `path_prefix` is a real prefix match

A route's `path_prefix` matches:

- the path itself, exactly, **and**
- any path that continues past it with additional segments.

```
path_prefix: /api/v1/profile

matches:     /api/v1/profile
matches:     /api/v1/profile/123
matches:     /api/v1/profile/123/settings
does NOT match: /api/v1/profiles       (segment boundary, not a text prefix)
```

Matching is segment-aware, not a raw string prefix: `/api/v1/profile` does
**not** match `/api/v1/profiles`. The trie stores every static segment
together with its terminating `/` and compares whole segments
(`RouteIndex.matchStatic`), so a route can never swallow a sibling segment
that merely shares a text prefix, nor can `/ab/c` reach a route `/abc`.

A trailing slash is equivalent to its absence (`/api/v1/profile/` matches
like `/api/v1/profile`), and `path_prefix: /` matches every path, which
makes it a catch-all for whatever no more specific route claims.

If two registered routes are both valid prefixes of the same request path
(e.g. `/api/v1` and `/api/v1/profile` for a request to
`/api/v1/profile/123`), the **more specific** one wins — see priority
order below.

### 1.2 Param and wildcard segments

- `:name` matches exactly one non-empty path segment. The name documents
  the segment; it is not used for matching.
- `*` or `*name` matches the remainder of the path, however many segments
  deep, including none (`/files/*` matches `/files`). It must be the last
  segment of a `path_prefix`.

Both are also treated as prefixes in the same sense as 1.1: a route
registered at a param/wildcard node is a valid fallback answer for any
path that continues past it, once nothing more specific exists deeper in
the tree.

### 1.3 Matching priority

For a given path, candidate routes are considered in this order, most
specific first:

1. **Static** children (exact segment text)
2. **Param** child (`:name`)
3. **Wildcard** child (`*name`)

This priority is applied **recursively at every segment**, and a match at
any tier includes both:

- routes registered further down that branch (more specific), and
- the route registered at that tier's own node, as a `path_prefix`
  fallback (1.1), if nothing more specific matched.

Priority order is a matter of *path structure only*. It says nothing about
method or headers — see 1.4.

### 1.4 Path structure vs. method/header predicates

The router does not stop at the first *structurally* matching route and
declare victory. If the most specific structural match doesn't support the
request's method or headers, the router falls back to the next
lower-priority candidate — exactly the way a human reading the route table
would expect a fallback route to work.

```
route A: static  "/users/me"     methods: [POST]
route B: param    "/users/:id"    methods: [GET]

GET /users/me
  -> static branch "me" matches structurally, but route A doesn't allow GET
  -> falls through to the param branch
  -> route B matches (GET is allowed) -> served by route B
```

Matching walks every priority tier in order and stops only once a route
satisfies **both** structure and predicates. A more specific structural match
that rejects the method or headers therefore never hides a valid fallback
route; in the example above, `GET /users/me` is served by route B.

## 2. Error responses

`proxy.Executor.ServeHTTP` returns exactly one of the outcomes listed below
for each request. This table is the contract: the code must not add, remove,
or reorder outcomes without updating this document in the same change.

| Status                      | Condition                                                                                                                                                                        | Notes                                                                                                                                                             |
|-----------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **503** Service Unavailable | No compiled view has been published (`view == nil`).                                                                                                                             | Startup failure; never a per-route condition.                                                                                                                     |
| **400** Bad Request         | The request path has no canonical form: it contains an encoded slash (`%2F`) or does not begin with `/` (see 1.0).                                                              | Checked before route matching. Rejection of other octets is specified but not yet enforced (see 1.0).                                                             |
| **405** Method Not Allowed  | (a) The request method is not a supported method (see `internal/httpmethod`); or (b) the path matched at least one route, but no matching candidate supports the request method. | Case (a) is checked before route matching. Case (b) is returned only when path matching succeeded. See 3 for the status code of case (a).                         |
| **404** Not Found           | No route's path structure matches the request path; or a path and method matched, but no candidate's header predicates were satisfied.                                           | See 2.1 for why an unsatisfied header predicate yields 404.                                                                                                       |
| **429** Too Many Requests   | A route matched (path, method, and headers), but its rate-limit policy rejected the request.                                                                                     | Checked before the first upstream attempt.                                                                                                                        |
| **504** Gateway Timeout     | A service `timeout`, a `retries.attempt_timeout` on the last attempt, the listener `request_timeout`, or a transport dial or response-header timeout expired.                    | Retries are attempted first while the service `timeout` allows.                                                                                                   |
| **502** Bad Gateway         | The upstream request failed for a reason other than a timeout.                                                                                                                   | Connection refused, connection reset, or protocol error.                                                                                                          |
| **200–5xx** (passthrough)   | The upstream responded.                                                                                                                                                          | Status, headers, and body are forwarded after hop-by-hop stripping and policy mutation. A 502, 503, or 504 response is passed through once retries are exhausted. |

### 2.1 Why a header mismatch is 404, not 405

If a route's path and method both match but its header predicates don't,
that route is invisible to the client for this request in every way that
matters — from the client's point of view, no such route exists. Returning
405 would incorrectly imply "this path exists, use a different method,"
which is not true here: the method was fine, an *unstated* header
requirement was not. This mirrors ordinary REST semantics: 405 is reserved
for "the resource exists, this verb isn't allowed on it," which requires
that resource to otherwise be visible to the caller.

### 2.2 Route label on metrics

**Status: the executor side is implemented; the metrics side is not.**

Every request that reaches a fully matched route (past the 503, 400, 404, and
405 checks above) is reported to `routelabel` with the compiled route's `Name`
before any subsequent 429/502/504 can occur, so that those responses can be
attributed to the correct route. `Name` is part of
`snapshot.CompiledRoute`'s contract (see `docs/policies.md`,
`routes: - name: example`) — if it is ever empty, that indicates a bug in
the loader/compiler, not a valid state, and the executor reports
`"unnamed-route"` rather than silently emitting a blank label.

The recorded value is not yet consumed. No code creates a `routelabel.Recorder`
and attaches it to the request context (`routelabel.NewContext` has no caller),
so `routelabel.FromContext` returns `nil` and the executor's call is a no-op.
The metrics middleware records only the request method and the response
status; the series `aegis_http_requests_total` and
`aegis_http_request_duration_seconds` carry no route label. Wiring the
recorder into the middleware is listed in the roadmap of
[ARCHITECTURE.md](../ARCHITECTURE.md).

## 3. Open items

- **Status code for unrecognized HTTP methods.** Such requests are currently
  answered with `405 Method Not Allowed` (case (a) in the table in section 2).
  RFC 9110 suggests `501 Not Implemented` for methods a server does not
  implement. Changing the status is a contract change and is not part of the
  current behavior.
