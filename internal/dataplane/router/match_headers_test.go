package router

import (
	"net/http"
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

func TestHeadersMatch(t *testing.T) {
	t.Run("no predicates always matches", func(t *testing.T) {
		if !HeadersMatch(nil, http.Header{}) {
			t.Fatal("expected true for empty predicates")
		}
	})

	t.Run("required header missing", func(t *testing.T) {
		preds := []snapshot.HeaderPredicate{{Name: "X-Req"}}
		if HeadersMatch(preds, http.Header{}) {
			t.Fatal("expected false when required header missing")
		}
	})

	t.Run("presence only predicate", func(t *testing.T) {
		preds := []snapshot.HeaderPredicate{{Name: "X-Req", AllowedValues: nil}}
		headers := http.Header{"X-Req": []string{"anything"}}
		if !HeadersMatch(preds, headers) {
			t.Fatal("expected true when required header is present")
		}
	})

	t.Run("allowed value matched from multiple request values", func(t *testing.T) {
		preds := []snapshot.HeaderPredicate{
			{Name: "X-Role", AllowedValues: []string{"admin", "owner"}},
		}
		headers := http.Header{"X-Role": []string{"user", "owner"}}
		if !HeadersMatch(preds, headers) {
			t.Fatal("expected true when one request value is allowed")
		}
	})

	t.Run("allowed values mismatch", func(t *testing.T) {
		preds := []snapshot.HeaderPredicate{
			{Name: "X-Role", AllowedValues: []string{"admin"}},
		}
		headers := http.Header{"X-Role": []string{"user"}}
		if HeadersMatch(preds, headers) {
			t.Fatal("expected false when no request value is allowed")
		}
	})
}

func TestAnyValueAllowed(t *testing.T) {
	t.Run("returns true when any pair matches", func(t *testing.T) {
		if !anyValueAllowed([]string{"a", "b"}, []string{"x", "b"}) {
			t.Fatal("expected true")
		}
	})

	t.Run("returns false when no values match", func(t *testing.T) {
		if anyValueAllowed([]string{"a"}, []string{"b", "c"}) {
			t.Fatal("expected false")
		}
	})

	t.Run("returns false for empty inputs", func(t *testing.T) {
		if anyValueAllowed(nil, []string{"x"}) {
			t.Fatal("expected false for empty request values")
		}
		if anyValueAllowed([]string{"x"}, nil) {
			t.Fatal("expected false for empty allowed values")
		}
	})
}
