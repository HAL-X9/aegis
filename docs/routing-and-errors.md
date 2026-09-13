# Routing semantics and error responses

This document is the single source of truth for two things that must stay
in lock-step: how a request path is matched against the route table, and
which HTTP status code `proxy.Executor` returns when. It replaces guesswork
in code comments — every status code below is produced from exactly one
place in `executor.go`.

## 1. Route matching semantics

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
**not** match `/api/v1/profiles`. The trie only ever compares whole path
segments (`matchStaticSegment`), so there is no way for a shorter route to
accidentally swallow a longer sibling segment that merely shares a text
prefix.

If two registered routes are both valid prefixes of the same request path
(e.g. `/api/v1` and `/api/v1/profile` for a request to
`/api/v1/profile/123`), the **more specific** one wins — see priority
order below.

### 1.2 Param and wildcard segments

- `:name` matches exactly one path segment and binds it as a parameter.
- `*name` matches the remainder of the path, however many segments deep.

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

Prior to this fix, the router returned only the most specific structural
match's candidate list and never considered lower-priority branches once
any structural match was found — which meant `GET /users/me` above
incorrectly produced a 404/405 instead of falling back to `/users/:id`.
This is now fixed: matching walks every priority tier, in order, and only
stops once a route satisfies **both** structure and predicates.

## 2. Error responses

`proxy.Executor.ServeHTTP` returns exactly one of the following outcomes
per request. This table is the contract; the code must not add, remove, or
reorder these without updating this document in the same change.

| Status                        | Condition                                                                                                                                                                       | Notes                                                                                                        |
|-------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------|
| **503** Service Unavailable   | The routing engine is unavailable (`engine == nil`).                                                                                                                            | Startup/config-reload failure; never a per-route condition.                                                  |
| **500** Internal Server Error | The upstream transport is unavailable (`transport == nil`).                                                                                                                     | Configuration/bootstrap error, not a request-time condition.                                                 |
| **400**/unsupported method    | The request method has no corresponding bit in `methodmask` (unrecognized HTTP method).                                                                                         | Returned before any route matching happens. Currently surfaced with a 405 status code — see open item below. |
| **404** Not Found             | No route's path structure matches the request path at all, **or** a route's path matched but no candidate route at any priority tier had headers that satisfied its predicates. | See 2.1 for why the "headers didn't match" case is 404, not 405.                                             |
| **405** Method Not Allowed    | At least one route's path matched the request, but none of the matching candidates — across *all* priority tiers — support the request method.                                  | Only returned when path matching succeeded somewhere and method is the sole blocker.                         |
| **429** Too Many Requests     | A route matched (path + method + headers) but its rate-limit policy rejected the request.                                                                                       |                                                                                                              |
| **502** Bad Gateway           | The upstream `RoundTrip` call failed.                                                                                                                                           | Upstream connectivity/timeout, not a client error.                                                           |
| **200-5xx** (passthrough)     | Upstream responded.                                                                                                                                                             | Status code, headers, and body are forwarded as received (after hop-header stripping and policy mutation).   |

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

Every request that reaches a fully matched route (past the 503/500/404/405
checks above) is labeled on `routelabel` with the compiled route's `Name`
before any subsequent 429/502 can occur, so those responses are still
attributed to the correct route in metrics. `Name` is part of
`snapshot.CompiledRoute`'s contract (see `docs/policies.md`,
`routes: - name: example`) — if it is ever empty, that indicates a bug in
the loader/compiler, not a valid state, and the executor labels it
`"unnamed-route"` rather than silently emitting a blank label.

## 3. Open items

- Unsupported HTTP methods currently return 405 before route matching even
  begins (`methodmask.MethodBit` failure). Consider 501 Not Implemented
  instead, since this is unrelated to any specific route's allowed
  methods — tracked separately, not changed in this pass to keep this fix
  scoped to the matching-priority and prefix-semantics bugs.