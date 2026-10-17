package proxy

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/dataplane/request"
	"github.com/HAL-X9/aegis/internal/httpmethod"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

// policyHeader is the request header that every route's header policy sets
// to its own name, which makes the policy that governed a request visible
// to the upstream.
const policyHeader = "X-Request-Id"

// pathPolicyConfig returns routes that differ in service and in header
// policy, so the route that governed a request can be read from two places
// that do not depend on each other: the upstream host and the policy header.
//
//	/public -> :9000, policy "public"
//	/admin  -> :9001, policy "admin"
//	/       -> :9003, policy "default"
func pathPolicyConfig() *snapshot.CompiledConfig {
	route := func(name string, service snapshot.ServiceID, prefix string) snapshot.CompiledRoute {
		return snapshot.CompiledRoute{
			Name:    name,
			Service: service,
			Match:   snapshot.CompiledMatch{PathPrefix: prefix, Methods: httpmethod.All},
			Policies: snapshot.CompiledRoutePolicies{
				RateLimitID: -1,
				Headers: snapshot.CompiledHeaders{Request: snapshot.CompiledHeadersPlan{Ops: []snapshot.HeaderInstruction{
					{HeaderID: snapshot.HeaderXRequestID, Op: snapshot.HeaderOpSet, Value: name},
				}}},
			},
		}
	}
	return &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{Items: []snapshot.CompiledService{
			{Name: "public", Upstream: testURL("http://upstream:9000")},
			{Name: "admin", Upstream: testURL("http://upstream:9001")},
			{Name: "default", Upstream: testURL("http://upstream:9003")},
		}},
		Routes: []snapshot.CompiledRoute{
			route("public", 0, "/public"),
			route("admin", 1, "/admin"),
			route("default", 2, "/"),
		},
	}
}

// pathOutcome is the three things the review asks to be checked together.
type pathOutcome struct {
	status int

	// routerPath is the path the router selects the route by: the value
	// ServeHTTP hands to Engine.Lookup, computed here from the parsed request
	// with the same function.
	routerPath string

	// upstreamEscaped is the path as it is written on the wire to the
	// upstream (the request-target of the outbound request line).
	upstreamEscaped string

	// upstreamHost and policy identify the route that governed the request.
	upstreamHost string
	policy       string
}

// servePath parses rawTarget the way the HTTP server does and serves it
// through a real Executor in front of a recording transport.
func servePath(t testing.TB, rawTarget string) (pathOutcome, *url.URL, bool) {
	t.Helper()

	req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
		"GET " + rawTarget + " HTTP/1.1\r\nHost: gateway\r\n\r\n")))
	if err != nil {
		return pathOutcome{}, nil, false
	}

	var (
		got       pathOutcome
		forwarded *url.URL
	)
	exec := newTestExecutor(t, buildTestEngine(t, pathPolicyConfig()), roundTripFunc(func(r *http.Request) (*http.Response, error) {
		forwarded = r.URL
		got.upstreamEscaped = r.URL.EscapedPath()
		got.upstreamHost = r.URL.Host
		got.policy = r.Header.Get(policyHeader)
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}))

	rec := httptest.NewRecorder()
	exec.ServeHTTP(rec, req)
	got.status = rec.Code

	if canonical, ok := request.CanonicalPath(req.URL); ok {
		got.routerPath = canonical
	}
	return got, forwarded, true
}

// wantPolicy is the oracle for which route governs a canonical path. It is
// written from the route table above and shares no code with the router.
func wantPolicy(routerPath string) (policy, host string) {
	first, _, _ := strings.Cut(strings.TrimPrefix(routerPath, "/"), "/")
	switch first {
	case "public":
		return "public", "upstream:9000"
	case "admin":
		return "admin", "upstream:9001"
	}
	return "default", "upstream:9003"
}

// TestExecutorPathPolicyAgreement pins, for each ambiguous request target,
// the path the router saw, the path the upstream received and the policy
// that was applied, and checks the cross-stage invariant on every accepted
// request: the policy that governs a request is the one its routing path
// selects, and the upstream is told exactly that path.
func TestExecutorPathPolicyAgreement(t *testing.T) {
	tests := []struct {
		name   string
		target string // raw request-target
		want   pathOutcome
	}{
		// Dot segments in every spelling are resolved before routing, so the
		// route is chosen by the resource that is actually addressed.
		{"dot segments", "/public/../admin", pathOutcome{200, "/admin", "/admin", "upstream:9001", "admin"}},
		{"encoded dot segments", "/public/%2e%2E/admin", pathOutcome{200, "/admin", "/admin", "upstream:9001", "admin"}},
		{"current directory", "/public/./x", pathOutcome{200, "/public/x", "/public/x", "upstream:9000", "public"}},
		{"empty segments", "//admin//x", pathOutcome{200, "/admin/x", "/admin/x", "upstream:9001", "admin"}},
		{"traversal past root", "/public/../../admin", pathOutcome{200, "/admin", "/admin", "upstream:9001", "admin"}},
		{"trailing dot segment", "/admin/x/..", pathOutcome{200, "/admin/", "/admin/", "upstream:9001", "admin"}},

		// An encoded slash is rejected: the router would split on it and the
		// upstream may not.
		{"encoded slash", "/public%2F..%2Fadmin", pathOutcome{status: 400}},
		{"encoded slash lower case", "/public%2fadmin", pathOutcome{status: 400}},
		{"encoded dot and slash", "/public/%2e%2e%2fadmin", pathOutcome{status: 400}},

		// Encoded octets that are not separators are decoded once by the
		// router and forwarded in a form that decodes to the same path once.
		{"encoded letter", "/ad%6din/x", pathOutcome{200, "/admin/x", "/ad%6din/x", "upstream:9001", "admin"}},
		{"upper case letter differs", "/%41dmin", pathOutcome{200, "/Admin", "/%41dmin", "upstream:9003", "default"}},
		{"encoded space", "/public/a%20b", pathOutcome{200, "/public/a b", "/public/a%20b", "upstream:9000", "public"}},

		// Double encoding stays encoded: the router sees a literal '%' and the
		// upstream receives "%25", so a single decode on the upstream yields
		// the same bytes the router saw.
		{"double encoded slash", "/public/%252fadmin", pathOutcome{200, "/public/%2fadmin", "/public/%252fadmin", "upstream:9000", "public"}},
		{"double encoded dot", "/public/%252e%252e/admin", pathOutcome{200, "/public/%2e%2e/admin", "/public/%252e%252e/admin", "upstream:9000", "public"}},
		{"double encoded backslash", "/public/%255Cadmin", pathOutcome{200, "/public/%5Cadmin", "/public/%255Cadmin", "upstream:9000", "public"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, ok := servePath(t, tt.target)
			if !ok {
				t.Fatalf("target %q is not accepted by net/http", tt.target)
			}
			if tt.want.status == http.StatusBadRequest {
				if got.status != http.StatusBadRequest || got.upstreamHost != "" {
					t.Fatalf("target %q: got %+v, want 400 and no upstream call", tt.target, got)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("target %q:\n got  %+v\n want %+v", tt.target, got, tt.want)
			}
			assertPathAgreement(t, tt.target, got)
		})
	}
}

// TestExecutorAmbiguousSeparatorsCurrentBehaviour records what happens today
// to the octets that some upstreams read as a path separator or terminator
// ('\', ';', NUL), docs/invariants.md I-004. The gateway routes and forwards
// them unchanged and consistently, so I-001 holds; what it does not do is
// stop an upstream that treats them specially from addressing a different
// resource than the route that was applied.
//
// Every case below is a request that I-004 requires to be answered 400. When
// I-004 is implemented these expectations flip to 400 and this test is
// replaced by a rejection test; until then it documents the gap rather than
// hiding it.
func TestExecutorAmbiguousSeparatorsCurrentBehaviour(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   pathOutcome
	}{
		// Tomcat strips ";params" from a segment before resolving "..", so it
		// reads this as /admin/x, while the route applied is "public".
		{"tomcat path parameter", "/public/..;/admin/x", pathOutcome{200, "/public/..;/admin/x", "/public/..;/admin/x", "upstream:9000", "public"}},
		{"semicolon", "/public/a;b", pathOutcome{200, "/public/a;b", "/public/a;b", "upstream:9000", "public"}},
		{"encoded semicolon", "/public/..%3B/admin", pathOutcome{200, "/public/..;/admin", "/public/..%3B/admin", "upstream:9000", "public"}},

		// IIS and other Windows stacks read '\' as '/', so they read this as
		// /admin, while the route applied is "public".
		{"raw backslash", `/public/..\admin`, pathOutcome{200, `/public/..\admin`, `/public/..%5Cadmin`, "upstream:9000", "public"}},
		{"encoded backslash", "/public/..%5Cadmin", pathOutcome{200, `/public/..\admin`, "/public/..%5Cadmin", "upstream:9000", "public"}},
		{"encoded backslash lower case", "/public/..%5cadmin", pathOutcome{200, `/public/..\admin`, "/public/..%5cadmin", "upstream:9000", "public"}},

		// A NUL terminates the path in C-based upstreams. Here the gateway
		// resolves "a\x00/.." itself, so the upstream never sees the NUL.
		{"NUL before dot segment", "/public/a%00/../admin", pathOutcome{200, "/public/admin", "/public/admin", "upstream:9000", "public"}},
		{"NUL in last segment", "/public/a%00b", pathOutcome{200, "/public/a\x00b", "/public/a%00b", "upstream:9000", "public"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, ok := servePath(t, tt.target)
			if !ok {
				t.Fatalf("target %q is not accepted by net/http", tt.target)
			}
			if got != tt.want {
				t.Fatalf("target %q:\n got  %+v\n want %+v", tt.target, got, tt.want)
			}
			// Even for these, the three stages agree with each other.
			assertPathAgreement(t, tt.target, got)
		})
	}
}

// assertPathAgreement checks the cross-stage invariant for an accepted
// request: the upstream is sent the path the router used, and the service and
// policy applied are the ones that path selects.
func assertPathAgreement(t testing.TB, target string, got pathOutcome) {
	t.Helper()

	decoded, err := url.PathUnescape(got.upstreamEscaped)
	if err != nil || decoded != got.routerPath {
		t.Fatalf("target %q: upstream path %q decodes to (%q, %v), router used %q",
			target, got.upstreamEscaped, decoded, err, got.routerPath)
	}
	wantPolicyName, wantHost := wantPolicy(got.routerPath)
	if got.policy != wantPolicyName || got.upstreamHost != wantHost {
		t.Fatalf("target %q: router path %q selects policy %q on %s, request got policy %q on %s",
			target, got.routerPath, wantPolicyName, wantHost, got.policy, got.upstreamHost)
	}
}

// FuzzExecutorPathPolicyAgreement checks, on arbitrary request targets, that
// no request is governed by a policy other than the one its routing path
// selects, and that the upstream is sent that same path. This is the
// invariant the review asks for: a request cannot escape its policy through a
// difference in how the path is read between stages.
func FuzzExecutorPathPolicyAgreement(f *testing.F) {
	for _, target := range []string{
		"/", "/public", "/public/x", "/admin", "/admin/x", "/adminx",
		"/public/../admin", "/public/%2e%2e/admin", "/public/%2E%2e/./admin",
		"//admin", "/public//../admin", "/public/..;/admin", "/public/..%3Badmin",
		`/public/..\admin`, "/public/..%5Cadmin", "/public/%5C../admin",
		"/public/%252e%252e/admin", "/public/%252fadmin", "/public%2F..%2Fadmin",
		"/public/a%00/../admin", "/%61dmin", "/%41dmin", "/admin%2e", "/admin%20",
		"/public/%", "/public/%zz", "/public/\xff/../admin", "/é/../admin",
		"/admin?x=/../public", "http://h/public/../admin",
	} {
		f.Add(target)
	}

	f.Fuzz(func(t *testing.T, target string) {
		got, _, ok := servePath(t, target)
		if !ok {
			t.Skip("not a request the server would accept")
		}
		if got.status == http.StatusBadRequest {
			if got.upstreamHost != "" {
				t.Fatalf("target %q: rejected request reached %s", target, got.upstreamHost)
			}
			return
		}
		if got.status != http.StatusOK {
			t.Fatalf("target %q: status %d, want 200 or 400", target, got.status)
		}
		assertPathAgreement(t, target, got)
	})
}
