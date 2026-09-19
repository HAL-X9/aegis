package request

import (
	"net"
	"net/http"
)

// SetForwardedHeaders adds the standard X-Forwarded-* hop information to
// an outbound header set, following the same convention as
// net/http/httputil.ReverseProxy:
//
//   - X-Forwarded-For gets the client's IP appended to any value already
//     present, so a chain of proxies accumulates the full path.
//   - X-Forwarded-Proto and X-Forwarded-Host are set only if not already
//     present: those describe the client-facing edge of the request, and
//     a later hop must not overwrite what an earlier proxy already
//     recorded.
//
// header must belong to the outbound (upstream) request — already cloned
// from the inbound request, see proxy.Executor.buildUpstreamRequest — so
// mutating it here never touches the original inbound r.Header. r is the
// original inbound request and is only read.
//
// This exists so "do we set X-Forwarded-*" is an explicit, single place
// to look, rather than an implicit gap in what the proxy forwards.
func SetForwardedHeaders(header http.Header, r *http.Request) {
	if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		if prior := header.Get("X-Forwarded-For"); prior != "" {
			header.Set("X-Forwarded-For", prior+", "+clientIP)
		} else {
			header.Set("X-Forwarded-For", clientIP)
		}
	}

	if header.Get("X-Forwarded-Proto") == "" {
		if r.TLS != nil {
			header.Set("X-Forwarded-Proto", "https")
		} else {
			header.Set("X-Forwarded-Proto", "http")
		}
	}

	if header.Get("X-Forwarded-Host") == "" && r.Host != "" {
		header.Set("X-Forwarded-Host", r.Host)
	}
}
