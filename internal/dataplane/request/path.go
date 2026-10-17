package request

import (
	"net/url"
	"path"
	"strings"
)

// CanonicalPath returns the path a request is both routed by and
// forwarded with, so the route that matches always governs the path the
// upstream receives.
//
// Dot segments ("." and "..") are resolved and empty segments ("//") are
// merged, per RFC 3986 §5.2.4; a trailing slash is kept. A path that is
// already canonical — virtually every request — is returned as-is,
// without allocating.
//
// ok is false when the path has no canonical form: it does not start with
// '/', or it contains an encoded slash ("%2F"). The router would split on
// such a slash while the upstream may not, so the two could disagree on
// which resource is addressed.
//
// It does not yet reject other ambiguous octets ('\', ';', NUL, '%') or
// their encoded forms: such bytes are currently routed and forwarded
// unchanged.
func CanonicalPath(u *url.URL) (canonical string, ok bool) {
	p := u.Path
	if !strings.HasPrefix(p, "/") || hasEncodedSlash(u.RawPath) {
		return "", false
	}
	if isCanonical(p) {
		return p, true
	}

	canonical = path.Clean(p)
	if canonical != "/" && endsInDirectory(p) {
		canonical += "/"
	}
	return canonical, true
}

// isCanonical reports whether p, which starts with '/', has no empty
// segments (other than the one after a trailing slash) and no dot segments.
func isCanonical(p string) bool {
	start := 1
	for i := 1; i <= len(p); i++ {
		if i < len(p) && p[i] != '/' {
			continue
		}
		switch p[start:i] {
		case "":
			if i < len(p) {
				return false
			}
		case ".", "..":
			return false
		}
		start = i + 1
	}
	return true
}

// endsInDirectory reports whether p's last segment is empty or a dot
// segment, which RFC 3986 resolves to a path ending in '/'
// ("/a/b/.." is "/a/"), unlike path.Clean.
func endsInDirectory(p string) bool {
	return strings.HasSuffix(p, "/") || strings.HasSuffix(p, "/.") || strings.HasSuffix(p, "/..")
}

// hasEncodedSlash reports whether rawPath contains "%2F" in either case.
func hasEncodedSlash(rawPath string) bool {
	for i := 0; i+2 < len(rawPath); i++ {
		if rawPath[i] == '%' && rawPath[i+1] == '2' && (rawPath[i+2] == 'F' || rawPath[i+2] == 'f') {
			return true
		}
	}
	return false
}
