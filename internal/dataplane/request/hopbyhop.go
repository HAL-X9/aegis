package request

import (
	"net/http"
	"strings"
)

// RemoveHopHeaders strips hop-by-hop headers from headers in place, per
// RFC 7230 §6.1. It must be called on both the outbound request (before
// proxying to upstream) and the inbound response (before writing it back
// to the client) — hop-by-hop headers are connection-scoped and must never
// be forwarded across either hop.
func RemoveHopHeaders(headers http.Header) {
	// The Connection header itself can name additional headers to remove,
	// and its value may be a single token or a comma-separated list
	// (e.g. "Connection: close, X-Custom-Hop"). Collect every named header
	// before deleting Connection so nothing is missed.
	for _, value := range headers.Values("Connection") {
		for name := range strings.SplitSeq(value, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				headers.Del(name)
			}
		}
	}
	headers.Del("Connection")

	for _, name := range standardHopByHop {
		headers.Del(name)
	}
}

var standardHopByHop = []string{
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection",
	"TE",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}
