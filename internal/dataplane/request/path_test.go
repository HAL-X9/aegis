package request

import (
	"net/url"
	"testing"
)

func TestCanonicalPath(t *testing.T) {
	tests := []struct {
		target string // request-target as a client sends it
		want   string
		wantOK bool
	}{
		{target: "/", want: "/", wantOK: true},
		{target: "/api/v1", want: "/api/v1", wantOK: true},
		{target: "/api/v1/", want: "/api/v1/", wantOK: true},
		{target: "/a%20b", want: "/a b", wantOK: true},
		{target: "/...", want: "/...", wantOK: true},

		{target: "//api", want: "/api", wantOK: true},
		{target: "/api//v1", want: "/api/v1", wantOK: true},
		{target: "/api/./v1", want: "/api/v1", wantOK: true},
		{target: "/public/../admin", want: "/admin", wantOK: true},
		{target: "/public/%2E%2E/admin", want: "/admin", wantOK: true},
		{target: "/../../etc", want: "/etc", wantOK: true},
		{target: "/a/b/..", want: "/a/", wantOK: true},
		{target: "/a/.", want: "/a/", wantOK: true},
		{target: "/a//", want: "/a/", wantOK: true},
		{target: "/..", want: "/", wantOK: true},

		{target: "/public%2F..%2Fadmin", wantOK: false},
		{target: "/a%2fb", wantOK: false},
		{target: "*", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			u, err := url.ParseRequestURI(tt.target)
			if err != nil {
				t.Fatalf("ParseRequestURI(%q): %v", tt.target, err)
			}

			got, ok := CanonicalPath(u)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("CanonicalPath(%q) = (%q, %v), want (%q, %v)", tt.target, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestCanonicalPathDoesNotAllocateForCanonicalPaths(t *testing.T) {
	u, err := url.ParseRequestURI("/api/v1/users/42/")
	if err != nil {
		t.Fatal(err)
	}

	allocs := testing.AllocsPerRun(100, func() {
		if _, ok := CanonicalPath(u); !ok {
			t.Fatal("canonical path rejected")
		}
	})
	if allocs != 0 {
		t.Fatalf("CanonicalPath allocated %v times per call, want 0", allocs)
	}
}
