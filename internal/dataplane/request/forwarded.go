package request

import (
	"net"
	"net/http"
)

// SetForwardedHeaders sets the standard X-Forwarded-* headers,
// overwriting any client-supplied values.
//
// Aegis is the public edge of the request path, so forwarded headers
// are derived only from the connection and original request. Inbound
// X-Forwarded-* values are never preserved or appended.
func SetForwardedHeaders(header http.Header, r *http.Request) {
	if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		header.Set("X-Forwarded-For", clientIP)
	} else {
		header.Del("X-Forwarded-For")
	}

	if r.TLS != nil {
		header.Set("X-Forwarded-Proto", "https")
	} else {
		header.Set("X-Forwarded-Proto", "http")
	}

	if r.Host != "" {
		header.Set("X-Forwarded-Host", r.Host)
	} else {
		header.Del("X-Forwarded-Host")
	}
}
