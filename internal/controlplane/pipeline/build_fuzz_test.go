package pipeline

import (
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/schema"
	"github.com/HAL-X9/aegis/internal/controlplane/validate"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

// FuzzPathPrefixBuild runs match.path_prefix through the whole control plane
// (validate, normalize, compile) and checks what a configuration author is
// promised: a prefix that is accepted means what it says.
//
// For every accepted prefix p:
//   - Build succeeds;
//   - the compiled prefix is p without one trailing slash, so normalization
//     neither changes nor drops anything the author wrote;
//   - the compiled prefix is itself a valid prefix (normalization is closed
//     under validation) and compiles to itself (idempotence);
//   - the route is reachable: a request path that spells out p, with a
//     parameter filled by "x" and a wildcard by nothing or by a longer
//     remainder, is offered the route by the index.
func FuzzPathPrefixBuild(f *testing.F) {
	for _, p := range []string{
		"/", "/a", "/a/", "/a/b", "/:id", "/:id/", "/a/:id/b", "/*", "/*rest",
		"/a/*", "/a/*files/", "/:a/*b", "/ab/abc/a", "/a.b/c.", "/...",
		"/ ", "/a ", "/a /", "/ /", "/\t", "/\u00a0", "/a\u2003", "/a\x00", "/é/è", "/%41", "/a;b",
	} {
		f.Add(p)
	}

	f.Fuzz(func(t *testing.T, prefix string) {
		validated, ok := validateOnePrefix(prefix)
		if !ok {
			t.Skip("rejected by validation")
		}

		compiled, err := Build(validated)
		if err != nil {
			t.Fatalf("prefix %q: validated but Build failed: %v", prefix, err)
		}
		if len(compiled.Routes) != 1 {
			t.Fatalf("prefix %q: compiled %d routes, want 1", prefix, len(compiled.Routes))
		}

		want := prefix
		if want != "/" {
			want = strings.TrimSuffix(want, "/")
		}
		got := compiled.Routes[0].Match.PathPrefix
		if got != want {
			t.Fatalf("prefix %q compiled to %q, want %q", prefix, got, want)
		}

		// Closed under validation, and a fixed point of the pipeline.
		again, ok := validateOnePrefix(got)
		if !ok {
			t.Fatalf("prefix %q: compiled form %q is rejected by validation", prefix, got)
		}
		recompiled, err := Build(again)
		if err != nil || recompiled.Routes[0].Match.PathPrefix != got {
			t.Fatalf("prefix %q: compiled form %q is not a fixed point: %q, %v",
				prefix, got, recompiled.Routes[0].Match.PathPrefix, err)
		}

		for _, path := range concretizations(got) {
			if !offers(compiled.Index, path, 0) {
				t.Fatalf("prefix %q (compiled %q) is unreachable: index does not offer it for %q", prefix, got, path)
			}
		}
	})
}

// validateOnePrefix validates a configuration with a single route using
// prefix. ok is false when validation rejects it.
func validateOnePrefix(prefix string) (validate.Validated, bool) {
	validated, err := validate.Validate(&schema.GatewayConfig{
		Services: schema.Services{
			"api": {Upstream: schema.Upstream{Scheme: "http", Host: "localhost", Port: 8080}},
		},
		Routes: []schema.Route{{
			Name:    "r",
			Service: "api",
			Match:   schema.Match{PathPrefix: prefix},
		}},
	})
	return validated, err == nil
}

// concretizations returns request paths that a canonical prefix must match:
// each literal segment as written, each ":name" as "x", and a trailing
// wildcard both as nothing and as a two-segment remainder. The prefix is
// already normalized, so it has no trailing slash.
func concretizations(prefix string) []string {
	if prefix == "/" {
		return []string{"/", "/x", "/x/y"}
	}

	segments := strings.Split(prefix[1:], "/")
	var wildcard bool
	for i, segment := range segments {
		switch {
		case strings.HasPrefix(segment, ":"):
			segments[i] = "x"
		case strings.HasPrefix(segment, "*"):
			wildcard = true
			segments = segments[:i]
		}
		if wildcard {
			break
		}
	}

	base := "/" + strings.Join(segments, "/")
	paths := []string{base, base + "/"}
	if base == "/" {
		paths = []string{"/"}
	}
	if !wildcard {
		return append(paths, strings.TrimSuffix(base, "/")+"/more")
	}
	return append(paths, strings.TrimSuffix(base, "/")+"/p/q")
}

// offers reports whether index.Lookup offers route id for path.
func offers(index snapshot.RouteIndex, path string, id snapshot.RouteID) bool {
	var found bool
	index.Lookup(path, func(candidates []snapshot.RouteID) bool {
		for _, c := range candidates {
			if c == id {
				found = true
			}
		}
		return false
	})
	return found
}
