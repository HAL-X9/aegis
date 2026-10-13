package request

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// headerSink keeps benchmarked headers on the heap, as they are in use.
var headerSink http.Header

func TestUpstreamHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://gateway.local/api", nil)
	r.RemoteAddr = "203.0.113.7:51234"
	r.Header.Set("Accept", "*/*")
	r.Header["X-Multi"] = []string{"a", "b"}
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-Host", "spoofed.example")

	h := UpstreamHeader(r)

	want := map[string][]string{
		"Accept":            {"*/*"},
		"X-Multi":           {"a", "b"},
		"X-Forwarded-For":   {"203.0.113.7"},
		"X-Forwarded-Proto": {"http"},
		"X-Forwarded-Host":  {"gateway.local"},
	}
	if len(h) != len(want) {
		t.Fatalf("header = %v, want %v", h, want)
	}
	for name, values := range want {
		if !slices.Equal(h[name], values) {
			t.Errorf("%s = %q, want %q", name, h[name], values)
		}
	}
}

func TestUpstreamHeaderIsIndependentOfRequest(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header["X-Multi"] = []string{"a", "b"}

	h := UpstreamHeader(r)
	h["X-Multi"][0] = "changed"
	h.Add("X-Multi", "c")
	h.Set("X-New", "v")

	if got := r.Header["X-Multi"]; !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("request X-Multi = %q, want unchanged", got)
	}
	if _, ok := r.Header["X-New"]; ok {
		t.Fatal("request gained X-New")
	}
}

func TestUpstreamHeaderTLSAndMissingConnectionInfo(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.TLS = &tls.ConnectionState{}
	r.RemoteAddr = "not-a-host-port"
	r.Host = ""
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	r.Header.Set("X-Forwarded-Host", "spoofed.example")

	h := UpstreamHeader(r)

	if got := h.Get("X-Forwarded-Proto"); got != "https" {
		t.Errorf("X-Forwarded-Proto = %q, want https", got)
	}
	if _, ok := h["X-Forwarded-For"]; ok {
		t.Errorf("X-Forwarded-For = %q, want removed", h["X-Forwarded-For"])
	}
	if _, ok := h["X-Forwarded-Host"]; ok {
		t.Errorf("X-Forwarded-Host = %q, want removed", h["X-Forwarded-Host"])
	}
}

func TestUpstreamHeaderAllocatesNoMoreThanClone(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:51234"
	r.Header.Set("Accept", "*/*")
	r.Header.Set("User-Agent", "test")
	r.Header.Set("X-Request-Id", "id")

	clone := testing.AllocsPerRun(100, func() { headerSink = r.Header.Clone() })
	upstream := testing.AllocsPerRun(100, func() { headerSink = UpstreamHeader(r) })

	if upstream > clone {
		t.Fatalf("UpstreamHeader allocated %v times, Header.Clone %v; forwarded headers must not add allocations", upstream, clone)
	}
}

func BenchmarkUpstreamHeader(b *testing.B) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.7:51234"
	r.Header.Set("Accept", "*/*")
	r.Header.Set("User-Agent", "benchmark")
	r.Header.Set("X-Request-Id", "id")

	b.ReportAllocs()
	for b.Loop() {
		headerSink = UpstreamHeader(r)
	}
}
