package validate

import (
	"strings"
	"testing"
	"unicode"
)

// FuzzValidatePathPrefix compares validatePathPrefix with referencePrefixOK,
// a second statement of the match.path_prefix grammar from
// docs/configuration.md written as a left-to-right scan instead of a
// split-and-switch, so the two cannot share a mistake in segment handling.
//
// The grammar here is the one enforced today. Rejection of ';' and '%'
// (docs/formal/canonical_path.md, D1 and D2) is not enforced yet; when it is,
// add those bytes to the rejected set below.
func FuzzValidatePathPrefix(f *testing.F) {
	for _, p := range []string{
		"/", "/a", "/a/", "/a/b", "/a//b", "/a/./b", "/a/../b", "//", "///",
		"/:id", "/:id/", "/:", "/:a-b", "/:a/:b", "/a/:id/b",
		"/*", "/*rest", "/*/", "/*a/b", "/a/*", "/a/*x/", "/a/*/b", "/**", "/*a-b",
		"/a?b", "/a#b", "/a;b", "/a%20b", "/a b", "/ ", "/a ", " /a", "", " ",
		"a", "/\x00", "/é", "/a\n", "/\u00a0", "/\u0085", "/\u200b", "/.", "/..", "/...", "/.a", "/a./b",
	} {
		f.Add(p)
	}

	f.Fuzz(func(t *testing.T, path string) {
		err := validatePathPrefix(path)
		if want := referencePrefixOK(path); (err == nil) != want {
			t.Fatalf("validatePathPrefix(%q) = %v, reference accepts = %v", path, err, want)
		}
	})
}

// referencePrefixOK reports whether path is a valid match.path_prefix.
//
//	path    = "/" | "/" segment { "/" segment } [ "/" ]
//	segment = literal | ":" name | "*" [ name ]     ("*" forms only last)
//	literal = non-empty, not "." or "..", not starting with ':' or '*'
//	name    = one or more of A-Z a-z 0-9 _
//
// "?" and "#" are not allowed anywhere. Whitespace and control characters are
// rejected so that normalization, which trims whitespace, cannot change what
// a prefix means (see FuzzPathPrefixBuild in internal/controlplane/pipeline).
func referencePrefixOK(path string) bool {
	if path == "" || path[0] != '/' {
		return false
	}
	for _, r := range path {
		switch {
		case r == '?' || r == '#':
			return false
		case unicode.IsSpace(r) || unicode.IsControl(r):
			return false
		}
	}
	if path == "/" {
		return true
	}

	body := strings.TrimSuffix(path[1:], "/")

	for pos := 0; ; {
		end := strings.IndexByte(body[pos:], '/')
		last := end < 0
		if last {
			end = len(body) - pos
		}
		segment := body[pos : pos+end]

		switch {
		case segment == "" || segment == "." || segment == "..":
			return false
		case segment[0] == ':':
			if !refName(segment[1:], false) {
				return false
			}
		case segment[0] == '*':
			if !last || !refName(segment[1:], true) {
				return false
			}
		}

		if last {
			return true
		}
		pos += end + 1
	}
}

// refName reports whether s is a valid parameter name; an empty s is valid
// only when emptyOK, which is the bare "*" wildcard.
func refName(s string, emptyOK bool) bool {
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
