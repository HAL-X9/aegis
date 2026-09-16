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
	// headers.Values("Connection") canonicalizes "Connection" on every
	// call despite it already being canonical; read the map directly.
	for _, value := range headers["Connection"] {
		for name := range strings.SplitSeq(value, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				// name comes from an arbitrary Connection header value —
				// not guaranteed canonical, so this Del must stay as-is.
				headers.Del(name)
			}
		}
	}
	delete(headers, "Connection")

	for _, name := range standardHopByHop {
		delete(headers, name)
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
