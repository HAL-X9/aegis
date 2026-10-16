package proxy

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/httpmethod"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

// FuzzExecutorPathAgreement checks the property that makes path
// canonicalization worth having (S5, S6 in docs/formal/canonical_path.md):
// the route that governs a request is the route its forwarded path names.
//
// The request is parsed from a raw HTTP/1.1 request line by net/http, as the
// server does, then served by a real Executor in front of a recording
// transport. Three services are told apart by upstream port, and the
// catch-all route gets the last one:
//
//	/admin    -> :9001
//	/internal -> :9002
//	/         -> :9003
//
// The oracle derives the expected service only from the first segment of the
// path the upstream actually received, so it fails if routing and forwarding
// ever read different paths.
func FuzzExecutorPathAgreement(f *testing.F) {
	for _, target := range []string{
		"/", "/admin", "/admin/x", "/internal/x", "/public/x", "/adminx",
		"//admin", "/./admin", "/public/../admin", "/public/%2E%2E/admin",
		"/admin/../internal", "/admin//../internal", "/%61dmin", "/%41dmin",
		"/admin%2F..%2Finternal", "/public%2fadmin", "/a%252Fb",
		"/admin/.", "/admin/..", "/internal/../../admin", "/a/b/../../../admin",
		"/admin?x=/../internal", "/ad%6din", "/é", "*", "http://h/admin/../x",
	} {
		f.Add(target)
	}

	routes := []snapshot.CompiledRoute{
		{Name: "admin", Service: 0, Match: snapshot.CompiledMatch{PathPrefix: "/admin", Methods: httpmethod.All}},
		{Name: "internal", Service: 1, Match: snapshot.CompiledMatch{PathPrefix: "/internal", Methods: httpmethod.All}},
		{Name: "catch-all", Service: 2, Match: snapshot.CompiledMatch{PathPrefix: "/", Methods: httpmethod.All}},
	}
	cfg := &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{Items: []snapshot.CompiledService{
			{Name: "admin", Upstream: testURL("http://upstream:9001")},
			{Name: "internal", Upstream: testURL("http://upstream:9002")},
			{Name: "default", Upstream: testURL("http://upstream:9003")},
		}},
		Routes: routes,
	}
	engine := buildTestEngine(f, cfg)

	f.Fuzz(func(t *testing.T, target string) {
		req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(
			"GET " + target + " HTTP/1.1\r\nHost: gateway\r\n\r\n")))
		if err != nil {
			t.Skip("not a request the server would accept")
		}

		var forwarded *url.URL
		calls := 0
		exec := newTestExecutor(t, engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			forwarded = r.URL
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
		}))

		rec := httptest.NewRecorder()
		exec.ServeHTTP(rec, req)

		// 400 exactly when the path cannot be read one way: it does not
		// start with '/', or its raw form hides a slash from the router.
		wantReject := !strings.HasPrefix(req.URL.Path, "/") ||
			strings.Contains(strings.ToLower(req.URL.RawPath), "%2f")
		if wantReject {
			if rec.Code != http.StatusBadRequest || calls != 0 {
				t.Fatalf("target %q: status %d, upstream calls %d, want 400 and no call", target, rec.Code, calls)
			}
			return
		}
		if rec.Code != http.StatusOK || calls != 1 {
			t.Fatalf("target %q: status %d, upstream calls %d, want 200 and one call", target, rec.Code, calls)
		}

		// S6: the upstream can read the forwarded path only one way.
		escaped := forwarded.EscapedPath()
		if strings.Contains(strings.ToLower(escaped), "%2f") {
			t.Fatalf("target %q: forwarded path %q hides a slash", target, escaped)
		}
		decoded, err := url.PathUnescape(escaped)
		if err != nil || decoded != forwarded.Path {
			t.Fatalf("target %q: forwarded path %q decodes to (%q, %v), want %q", target, escaped, decoded, err, forwarded.Path)
		}

		// The forwarded path has no segment that is "." or "..", so no
		// upstream can resolve it to a different resource than the gateway did.
		for _, segment := range strings.Split(forwarded.Path, "/") {
			if segment == "." || segment == ".." {
				t.Fatalf("target %q: forwarded path %q has dot segment %q", target, forwarded.Path, segment)
			}
		}

		// S5: the service that served the request is the one the forwarded
		// path's first segment selects.
		first, _, _ := strings.Cut(strings.TrimPrefix(forwarded.Path, "/"), "/")
		wantHost := "upstream:9003"
		switch first {
		case "admin":
			wantHost = "upstream:9001"
		case "internal":
			wantHost = "upstream:9002"
		}
		if forwarded.Host != wantHost {
			t.Fatalf("target %q: forwarded to %s with path %q, want %s", target, forwarded.Host, forwarded.Path, wantHost)
		}

		// The query string is forwarded untouched.
		if forwarded.RawQuery != req.URL.RawQuery {
			t.Fatalf("target %q: forwarded query %q, want %q", target, forwarded.RawQuery, req.URL.RawQuery)
		}
	})
}
