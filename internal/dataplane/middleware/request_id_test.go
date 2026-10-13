package middleware

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

var uuidV7Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// serveRequestID runs req through RequestID and returns the X-Request-Id
// values the next handler saw and the one echoed to the client.
func serveRequestID(t *testing.T, req *http.Request) (forwarded []string, echoed string) {
	t.Helper()

	handler := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		forwarded = r.Header.Values("X-Request-Id")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return forwarded, rec.Header().Get("X-Request-Id")
}

func TestRequestIDKeepsValidClientID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "client-trace:42/a=b")

	forwarded, echoed := serveRequestID(t, req)

	if len(forwarded) != 1 || forwarded[0] != "client-trace:42/a=b" {
		t.Fatalf("forwarded X-Request-Id = %q, want the client's ID", forwarded)
	}
	if echoed != "client-trace:42/a=b" {
		t.Fatalf("echoed X-Request-Id = %q, want the client's ID", echoed)
	}
}

func TestRequestIDReplacesInvalidClientID(t *testing.T) {
	tests := map[string][]string{
		"absent":            nil,
		"empty":             {""},
		"too long":          {strings.Repeat("a", maxRequestIDLen+1)},
		"space":             {"two words"},
		"control character": {"id\x00forged"},
		"non-ASCII":         {"ид"},
		"repeated":          {"first", "second"},
	}

	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for _, v := range values {
				req.Header.Add("X-Request-Id", v)
			}

			forwarded, echoed := serveRequestID(t, req)

			if !uuidV7Pattern.MatchString(echoed) {
				t.Fatalf("echoed X-Request-Id = %q, want a generated UUIDv7", echoed)
			}
			if len(forwarded) != 1 || forwarded[0] != echoed {
				t.Fatalf("forwarded X-Request-Id = %q, want exactly the echoed %q", forwarded, echoed)
			}
		})
	}
}

func TestRequestIDKeepsMaxLengthClientID(t *testing.T) {
	id := strings.Repeat("a", maxRequestIDLen)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", id)

	if _, echoed := serveRequestID(t, req); echoed != id {
		t.Fatalf("echoed X-Request-Id = %q, want the %d-byte client ID", echoed, maxRequestIDLen)
	}
}

func TestRequestIDDoesNotAllocateForValidClientID(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "client-trace-42")
	rec := httptest.NewRecorder()

	allocs := testing.AllocsPerRun(100, func() {
		handler.ServeHTTP(rec, req)
	})
	if allocs != 0 {
		t.Fatalf("RequestID allocated %v times per request, want 0", allocs)
	}
}

func TestNewRequestID(t *testing.T) {
	before := time.Now().UnixMilli()
	id := newRequestID()
	after := time.Now().UnixMilli()

	if !uuidV7Pattern.MatchString(id) {
		t.Fatalf("newRequestID() = %q, want a UUIDv7", id)
	}

	timestamp, err := hex.DecodeString(id[0:8] + id[9:13])
	if err != nil {
		t.Fatalf("decode timestamp of %q: %v", id, err)
	}
	var ms int64
	for _, b := range timestamp {
		ms = ms<<8 | int64(b)
	}
	if ms < before || ms > after {
		t.Fatalf("timestamp = %d ms, want within [%d, %d]", ms, before, after)
	}

	if other := newRequestID(); other == id {
		t.Fatalf("two calls returned the same ID %q", id)
	}
}

func BenchmarkNewRequestID(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = newRequestID()
	}
}
