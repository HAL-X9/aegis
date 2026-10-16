package request

import (
	"net/url"
	"strings"
	"testing"
)

// FuzzCanonicalPath checks CanonicalPath against the properties of
// docs/formal/canonical_path.md (S1 to S3 and the rejection rule of §3) on
// request targets parsed the way the HTTP server parses them.
//
// The oracle never derives the expected value from CanonicalPath. It uses
// referenceCanonical, the segment merge of decision D3 followed by a direct
// transcription of RFC 3986 §5.2.4, written without path.Clean. A mismatch
// is either a bug in CanonicalPath or a gap in the specification.
func FuzzCanonicalPath(f *testing.F) {
	for _, target := range []string{
		"/", "/api/v1", "/api/v1/", "/a%20b", "/...", "/a.b/.c/d.",
		"//api", "/api//v1", "/api/./v1", "/public/../admin", "/public/%2E%2E/admin",
		"/../../etc", "/a/b/..", "/a/.", "/a//", "/..", "/.", "/./", "//", "///",
		"/a/b/../../..", "/a/../b/", "/%2e", "/%2e%2e/", "/a/%2e%2E/b",
		"/public%2F..%2Fadmin", "/a%2fb", "/a%2Fb", "/a%252Fb", "/%2",
		"/a?x=/../y", "/a%00b", "/a%5Cb", "/a;b", "/\xff", "/é/../x",
		"*", "http://example.com/a/../b",
	} {
		f.Add(target)
	}

	f.Fuzz(func(t *testing.T, target string) {
		u, err := url.ParseRequestURI(target)
		if err != nil {
			t.Skip("not a request target the server would accept")
		}

		got, ok := CanonicalPath(u)

		// Rejection rule: no leading '/', or an encoded slash in the raw path.
		wantOK := strings.HasPrefix(u.Path, "/") &&
			!strings.Contains(strings.ToLower(u.RawPath), "%2f")
		if ok != wantOK {
			t.Fatalf("target %q: ok = %v, want %v (Path=%q RawPath=%q)", target, ok, wantOK, u.Path, u.RawPath)
		}
		if !ok {
			if got != "" {
				t.Fatalf("target %q: rejected path returned %q, want empty", target, got)
			}
			return
		}

		// Differential: the result is C(P) as defined by the specification.
		if want := referenceCanonical(u.Path); got != want {
			t.Fatalf("target %q: CanonicalPath(%q) = %q, reference = %q", target, u.Path, got, want)
		}

		// S1 and S2, checked directly on the output.
		if !strings.HasPrefix(got, "/") {
			t.Fatalf("target %q: canonical %q does not start with '/'", target, got)
		}
		segments := strings.Split(got[1:], "/")
		for i, segment := range segments {
			if segment == "." || segment == ".." {
				t.Fatalf("target %q: canonical %q has dot segment (S1)", target, got)
			}
			if segment == "" && i != len(segments)-1 {
				t.Fatalf("target %q: canonical %q has an empty internal segment (S2)", target, got)
			}
		}

		// S3: canonicalizing a canonical path changes nothing.
		again, ok := CanonicalPath(&url.URL{Path: got})
		if !ok || again != got {
			t.Fatalf("target %q: not idempotent: %q -> (%q, %v)", target, got, again, ok)
		}

		// Canonicalization only ever shortens a path: it removes bytes and
		// adds at most the trailing '/' that RFC 3986 gives a final dot segment.
		if len(got) > len(u.Path)+1 {
			t.Fatalf("target %q: canonical %q is longer than input %q", target, got, u.Path)
		}
	})
}

// referenceCanonical computes RemoveDotSegments(Collapse(p)) for a path that
// starts with '/'. See docs/formal/canonical_path.md §2.
//
// The merge comes first, so "/a//../b" is "/b": the ".." removes "a", as it
// does after path.Clean and in servers that merge slashes before resolving
// dots. Resolving dots first would let ".." remove the empty segment and
// give "/a/b", which is the RFC 3986 reading of "//" but not decision D3.
func referenceCanonical(p string) string {
	return removeDotSegments(collapse(p))
}

// removeDotSegments is the input/output buffer algorithm of RFC 3986
// §5.2.4, step for step.
func removeDotSegments(input string) string {
	var output []string // output buffer as a stack of "/segment" or "/" pieces

	popLast := func() {
		if len(output) > 0 {
			output = output[:len(output)-1]
		}
	}

	for input != "" {
		switch {
		case strings.HasPrefix(input, "../"): // 2A
			input = input[3:]
		case strings.HasPrefix(input, "./"): // 2A
			input = input[2:]
		case strings.HasPrefix(input, "/./"): // 2B
			input = "/" + input[3:]
		case input == "/.": // 2B
			input = "/"
		case strings.HasPrefix(input, "/../"): // 2C
			input = "/" + input[4:]
			popLast()
		case input == "/..": // 2C
			input = "/"
			popLast()
		case input == "." || input == "..": // 2D
			input = ""
		default: // 2E: move the first path segment, with its leading '/', to the output
			end := strings.IndexByte(input[1:], '/')
			if end < 0 {
				end = len(input)
			} else {
				end++
			}
			output = append(output, input[:end])
			input = input[end:]
		}
	}
	return strings.Join(output, "")
}

// collapse removes every empty segment of p except a final one, which
// stands for a trailing '/'.
func collapse(p string) string {
	parts := strings.Split(p[1:], "/")
	last := parts[len(parts)-1]

	kept := make([]string, 0, len(parts))
	for _, part := range parts[:len(parts)-1] {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return "/" + strings.Join(append(kept, last), "/")
}

// TestReferenceCanonical pins the reference implementation to the RFC 3986
// §5.4 examples that apply to absolute paths, so the fuzz oracle is itself
// checked against an external source rather than against CanonicalPath.
func TestReferenceCanonical(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		// RFC 3986 §5.2.4 worked example and §5.4.2 abnormal examples, merged
		// where D3 applies.
		{"/a/b/c/./../../g", "/a/g"},
		{"/mid/content=5/../6", "/mid/6"},
		{"/../g", "/g"},
		{"/./g", "/g"},
		{"/g/.", "/g/"},
		{"/g/..", "/"},
		{"/a/b/..", "/a/"},
		{"/a/./b/../c/.", "/a/c/"},

		// D3: empty segments are merged before dot segments are resolved.
		{"//", "/"},
		{"///a//b///", "/a/b/"},
		{"/a//../b", "/b"},
		{"/0//..", "/"},
		{"/a/b//..//c", "/a/c"},
		{"/", "/"},
	} {
		if got := referenceCanonical(tt.in); got != tt.want {
			t.Errorf("referenceCanonical(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
