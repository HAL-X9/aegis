package request

import (
	"net"
	"net/http"
)

// forwardedHeaders are the headers UpstreamHeader derives from the
// connection.
var forwardedHeaders = [...]string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host"}

// UpstreamHeader returns a copy of r.Header for the upstream request, with
// X-Forwarded-For, X-Forwarded-Proto and X-Forwarded-Host derived from r's
// connection and Host, replacing any client-supplied values.
//
// The three headers are always replaced, never trusted, so a client cannot
// forge its address, scheme or host as seen by the gateway. Other headers that
// can carry the same information (Forwarded, X-Real-IP, X-Original-URL, ...)
// are copied unchanged.
//
// The copy is built in one pass with a single map and a single backing
// array for all values, including the forwarded ones.
func UpstreamHeader(r *http.Request) http.Header {
	values, keys := len(forwardedHeaders), len(r.Header)
	for _, vv := range r.Header {
		values += len(vv)
	}
	// An exact key count keeps small headers in a single map group.
	for _, name := range forwardedHeaders {
		if _, ok := r.Header[name]; !ok {
			keys++
		}
	}
	backing := make([]string, values)
	h := make(http.Header, keys)

	for name, vv := range r.Header {
		c := copy(backing, vv)
		h[name] = backing[:c:c]
		backing = backing[c:]
	}

	set := func(name, value string) {
		backing[0] = value
		h[name] = backing[:1:1]
		backing = backing[1:]
	}

	if clientIP, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		set("X-Forwarded-For", clientIP)
	} else {
		delete(h, "X-Forwarded-For")
	}

	if r.TLS != nil {
		set("X-Forwarded-Proto", "https")
	} else {
		set("X-Forwarded-Proto", "http")
	}

	if r.Host != "" {
		set("X-Forwarded-Host", r.Host)
	} else {
		delete(h, "X-Forwarded-Host")
	}

	return h
}
