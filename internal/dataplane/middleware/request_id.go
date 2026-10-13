package middleware

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"time"
)

// requestIDHeader is X-Request-ID in canonical form, so it can be used as
// an http.Header map key directly.
const requestIDHeader = "X-Request-Id"

// maxRequestIDLen bounds the length of a client-supplied request ID.
const maxRequestIDLen = 128

// RequestID gives every request exactly one valid X-Request-Id and echoes
// it in the response.
//
// A client-supplied ID is kept when it is a single value of 1 to 128
// visible ASCII characters, so a caller's ID can be followed through the
// gateway. Anything else — absent, repeated, oversized, or containing
// spaces or control characters that could forge log lines — is replaced
// with a fresh UUIDv7.
//
// The ID is written into r.Header, deliberately departing from net/http's
// advice not to modify the request: that is how it reaches the upstream
// (the proxy forwards request headers) and every later handler, at no
// cost when the client's ID is kept.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header[requestIDHeader]
		if !isValidRequestID(id) {
			id = []string{newRequestID()}
			r.Header[requestIDHeader] = id
		}
		// Request and response share the one-element slice. That is safe:
		// header values are replaced or appended to, never written in place.
		w.Header()[requestIDHeader] = id

		next.ServeHTTP(w, r)
	})
}

func isValidRequestID(values []string) bool {
	if len(values) != 1 {
		return false
	}
	id := values[0]
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] < '!' || id[i] > '~' {
			return false
		}
	}
	return true
}

// newRequestID returns a fresh UUIDv7 (RFC 9562): a 48-bit Unix
// millisecond timestamp followed by random bits, so IDs sort by creation
// time. It is built here rather than with github.com/google/uuid, whose
// NewV7 serializes all callers on a global mutex and allocates.
func newRequestID() string {
	var id [16]byte
	binary.BigEndian.PutUint64(id[:8], uint64(time.Now().UnixMilli())<<16)
	// crypto/rand.Read never returns an error: it crashes the program
	// instead if the operating system fails to provide randomness.
	_, _ = rand.Read(id[6:])
	id[6] = id[6]&0x0f | 0x70 // version 7
	id[8] = id[8]&0x3f | 0x80 // RFC 9562 variant

	var text [36]byte
	hex.Encode(text[0:8], id[0:4])
	text[8] = '-'
	hex.Encode(text[9:13], id[4:6])
	text[13] = '-'
	hex.Encode(text[14:18], id[6:8])
	text[18] = '-'
	hex.Encode(text[19:23], id[8:10])
	text[23] = '-'
	hex.Encode(text[24:36], id[10:16])
	return string(text[:])
}
