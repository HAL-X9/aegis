package request

import (
	"net/http"
	"testing"
)

func TestRemoveHopHeaders(t *testing.T) {
	h := http.Header{
		"Connection": {"x-custom, , X-Other", "X-Third"},
		"X-Custom":   {"1"},
		"X-Other":    {"2"},
		"X-Third":    {"3"},
		"Keep-Alive": {"timeout=5"},
		"Upgrade":    {"websocket"},
		"Accept":     {"*/*"},
	}

	RemoveHopHeaders(h)

	if len(h) != 1 || h.Get("Accept") != "*/*" {
		t.Fatalf("header = %v, want only Accept", h)
	}
}
