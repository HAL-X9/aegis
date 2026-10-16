package compile

import (
	"slices"
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/snapshot"
)

// FuzzIndexLookup builds the route index from a fuzzed set of path prefixes
// and checks every lookup against a model that knows nothing about radix
// trees: each route is matched on its own, segment by segment.
//
// For a canonical request path the model fixes three things the index must
// agree with:
//
//   - which routes are offered: exactly the routes whose prefix matches, each
//     once (segment-boundary prefix semantics, ":name" matches one segment,
//     a trailing "*" matches the rest, possibly nothing);
//   - in which order: at each segment a static edge beats ":name", which beats
//     "*", which beats a route that ends here; when the path itself ends,
//     routes that end exactly here come before a wildcard;
//   - that offering stops as soon as visit returns true.
//
// Arbitrary bytes are folded into a small alphabet so that inputs share
// prefixes, which is what splits radix edges, and so that every input is a
// structurally interesting prefix set rather than noise.
func FuzzIndexLookup(f *testing.F) {
	f.Add("/a\n/a/b\n/ab\n/abc\n/a/:x\n/a/*\n/:x\n/*\n/", "/a/b")
	f.Add("/ab/c\n/abc\n/a", "/abc")
	f.Add("/a/:x/c\n/a/b/c\n/a/*r\n/a", "/a/b/c")
	f.Add("/*\n/", "/")
	f.Add("/a/*\n/a", "/a")
	f.Add("/:a\n/:b\n/c", "/c")
	f.Add("/a/b/c/d\n/a/b\n/a/b/c", "/a/b/c/")
	f.Add("/\xff/a\n/\xff\n/a\xff", "/\xff/a")

	f.Fuzz(func(t *testing.T, spec, rawPath string) {
		var routes []snapshot.CompiledRoute
		var models []modelRoute
		for _, line := range strings.SplitN(spec, "\n", maxFuzzRoutes+1) {
			if len(routes) == maxFuzzRoutes {
				break
			}
			prefix := fold(line)
			model, ok := parseModelRoute(prefix)
			if !ok {
				continue
			}
			routes = append(routes, snapshot.CompiledRoute{
				Name:  "r",
				Match: snapshot.CompiledMatch{PathPrefix: model.prefix},
			})
			models = append(models, model)
		}

		index, err := Index(routes)
		if err != nil {
			t.Fatalf("Index(%q) failed: %v", spec, err)
		}

		path := canonicalFuzzPath(fold(rawPath))
		segments := pathSegments(path)

		want := expectedOrder(models, segments)

		var got []snapshot.RouteID
		index.Lookup(path, func(candidates []snapshot.RouteID) bool {
			got = append(got, candidates...)
			return false
		})
		if !slices.Equal(got, want) {
			t.Fatalf("routes %q, path %q:\n got  %v\n want %v", prefixes(models), path, got, want)
		}

		// Early exit: visit returning true ends the lookup after one set.
		calls := 0
		index.Lookup(path, func([]snapshot.RouteID) bool {
			calls++
			return true
		})
		if wantCalls := min(len(want), 1); calls != wantCalls {
			t.Fatalf("routes %q, path %q: visit called %d times after returning true", prefixes(models), path, calls)
		}
	})
}

// maxFuzzRoutes and maxFuzzPathLen keep one execution cheap.
const (
	maxFuzzRoutes  = 12
	maxFuzzPathLen = 48
)

// alphabet is the set of bytes fuzzed routes and paths are built from. It
// has three letters so prefixes overlap, the three route metacharacters,
// the characters that are legal in a name, a non-ASCII byte that sorts after
// every other, and '/' for structure. Dot is left out: canonical paths and
// valid prefixes have no dot segments, so it adds no case.
const alphabet = "abc/:*_1\xff"

// fold maps every byte of s into alphabet, leaving alphabet bytes alone so
// that seeds read as written. '\n' is kept: it separates routes in a spec.
func fold(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c != '\n' && strings.IndexByte(alphabet, c) < 0 {
			b[i] = alphabet[int(c)%len(alphabet)]
		}
	}
	return string(b)
}

type segKind int

const (
	segStatic segKind = iota
	segParam
	segWildcard
)

type modelSeg struct {
	kind segKind
	text string // the literal for segStatic
}

// modelRoute is a route prefix parsed the plain way.
type modelRoute struct {
	prefix string // normalized: no trailing slash unless "/"
	segs   []modelSeg
}

// parseModelRoute parses prefix as the control plane would have validated
// and normalized it, reporting false for anything validation rejects. It is
// restricted to what the alphabet can produce: no dot, no whitespace.
func parseModelRoute(prefix string) (modelRoute, bool) {
	if !strings.HasPrefix(prefix, "/") {
		return modelRoute{}, false
	}
	if prefix != "/" {
		prefix = strings.TrimSuffix(prefix, "/")
	}
	if prefix == "/" {
		return modelRoute{prefix: "/"}, true
	}

	parts := strings.Split(prefix[1:], "/")
	segs := make([]modelSeg, len(parts))
	for i, part := range parts {
		switch {
		case part == "":
			return modelRoute{}, false
		case part[0] == ':':
			if !isName(part[1:], false) {
				return modelRoute{}, false
			}
			segs[i] = modelSeg{kind: segParam}
		case part[0] == '*':
			if i != len(parts)-1 || !isName(part[1:], true) {
				return modelRoute{}, false
			}
			segs[i] = modelSeg{kind: segWildcard}
		default:
			segs[i] = modelSeg{kind: segStatic, text: part}
		}
	}
	return modelRoute{prefix: prefix, segs: segs}, true
}

func isName(s string, emptyOK bool) bool {
	if s == "" {
		return emptyOK
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
		default:
			return false
		}
	}
	return true
}

// canonicalFuzzPath turns s into a canonical request path: it starts with
// '/', has no empty segments, and is cut to maxFuzzPathLen. A trailing slash
// is kept, since requests may carry one.
func canonicalFuzzPath(s string) string {
	s = strings.ReplaceAll(s, "\n", "/")
	if len(s) > maxFuzzPathLen {
		s = s[:maxFuzzPathLen]
	}
	trailing := strings.HasSuffix(s, "/")

	var kept []string
	for _, part := range strings.Split(s, "/") {
		if part != "" {
			kept = append(kept, part)
		}
	}
	path := "/" + strings.Join(kept, "/")
	if trailing && len(kept) > 0 {
		path += "/"
	}
	return path
}

// pathSegments splits a canonical path; a trailing slash adds no segment,
// because the index treats "/a/" and "/a" alike.
func pathSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// expectedOrder returns the route IDs the index must offer for a path with
// the given segments, in offer order.
func expectedOrder(models []modelRoute, segments []string) []snapshot.RouteID {
	type match struct {
		id  snapshot.RouteID
		key []int
	}
	var matches []match

	for id, route := range models {
		if key, ok := route.matchKey(segments); ok {
			matches = append(matches, match{snapshot.RouteID(id), key})
		}
	}

	slices.SortStableFunc(matches, func(a, b match) int {
		if c := slices.Compare(a.key, b.key); c != 0 {
			return c
		}
		return int(a.id) - int(b.id)
	})

	var ids []snapshot.RouteID
	for _, m := range matches {
		ids = append(ids, m.id)
	}
	return ids
}

// Ranks of how a route consumes one path segment, and how it ends. Smaller
// is offered first. The ranks of the two ways to end swap when the path has
// no segments left at that depth.
const (
	rankStatic = 0
	rankParam  = 1
	rankFirst  = 2 // wildcard while segments remain; exact end once none do
	rankSecond = 3 // exact end while segments remain; wildcard once none do
)

// matchKey reports whether the route matches a path with the given
// segments, and returns the sort key that orders it among the other matches.
func (r modelRoute) matchKey(segments []string) ([]int, bool) {
	key := make([]int, 0, len(r.segs)+1)

	for depth, seg := range r.segs {
		exhausted := depth == len(segments)

		switch seg.kind {
		case segWildcard:
			// Matches whatever is left, including nothing.
			if exhausted {
				return append(key, rankSecond), true
			}
			return append(key, rankFirst), true
		case segStatic:
			if exhausted || segments[depth] != seg.text {
				return nil, false
			}
			key = append(key, rankStatic)
		case segParam:
			if exhausted {
				return nil, false
			}
			key = append(key, rankParam)
		}
	}

	if len(r.segs) == len(segments) {
		return append(key, rankFirst), true
	}
	return append(key, rankSecond), true
}

func prefixes(models []modelRoute) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = m.prefix
	}
	return out
}
