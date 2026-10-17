package snapshot

import (
	"net/textproto"
	"testing"
)

// TestWellKnownHeaderTable guards the single table behind the fixed header
// IDs: every ID below HeaderDynamicStart other than HeaderUnknown is named
// exactly once, in canonical form, and the two lookup directions agree.
func TestWellKnownHeaderTable(t *testing.T) {
	seen := make(map[string]HeaderID)

	for id := HeaderUnknown + 1; id < HeaderDynamicStart; id++ {
		name := WellKnownHeaderName(id)
		if name == "" {
			t.Errorf("header ID %d has no name; add it to wellKnownHeaderNames", id)
			continue
		}
		if canonical := textproto.CanonicalMIMEHeaderKey(name); name != canonical {
			t.Errorf("header ID %d is named %q, want the canonical form %q", id, name, canonical)
		}
		if other, dup := seen[name]; dup {
			t.Errorf("header name %q is used by both ID %d and ID %d", name, other, id)
		}
		seen[name] = id

		got, ok := WellKnownHeaderID(name)
		if !ok || got != id {
			t.Errorf("WellKnownHeaderID(%q) = (%d, %v), want (%d, true)", name, got, ok, id)
		}
	}

	if len(wellKnownHeaderIDs) != len(seen) {
		t.Errorf("reverse table has %d entries, want %d", len(wellKnownHeaderIDs), len(seen))
	}
}

func TestWellKnownHeaderIDIgnoresCase(t *testing.T) {
	for _, name := range []string{"x-request-id", "X-REQUEST-ID", "X-Request-ID"} {
		if id, ok := WellKnownHeaderID(name); !ok || id != HeaderXRequestID {
			t.Errorf("WellKnownHeaderID(%q) = (%d, %v), want (%d, true)", name, id, ok, HeaderXRequestID)
		}
	}
}

func TestWellKnownHeaderLookupsRejectOthers(t *testing.T) {
	if _, ok := WellKnownHeaderID("X-Custom"); ok {
		t.Error("WellKnownHeaderID accepted a custom header")
	}
	for _, id := range []HeaderID{HeaderUnknown, HeaderDynamicStart, HeaderDynamicStart + 5} {
		if name := WellKnownHeaderName(id); name != "" {
			t.Errorf("WellKnownHeaderName(%d) = %q, want empty", id, name)
		}
	}
}
