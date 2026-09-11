package compile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

// ──────────────────────────── Policy ──────────────────────────────────────

func TestPolicy_nilReturnsError(t *testing.T) {
	_, err := Policies(nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "config is nil") {
		t.Fatalf("error does not mention nil config: %v", err)
	}
}

func TestPolicy_emptyHeaders(t *testing.T) {
	out, err := Policies(&ir.Policies{
		Headers: map[string]ir.Headers{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Headers) != 0 {
		t.Fatalf("expected empty compiled headers, got %d", len(out.Headers))
	}
}

func TestPolicy_singlePolicyRequestSet(t *testing.T) {
	out, err := Policies(&ir.Policies{
		Headers: map[string]ir.Headers{
			"ct": {
				Request: ir.HeadersOps{
					Set: map[string]string{"Content-Type": "application/json"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Headers) != 1 {
		t.Fatalf("expected 1 compiled header set, got %d", len(out.Headers))
	}

	plan := out.Headers[0].Request
	if len(plan.Ops) != 1 {
		t.Fatalf("expected 1 request op, got %d", len(plan.Ops))
	}
	op := plan.Ops[0]
	if op.Op != snapshot.HeaderOpSet {
		t.Errorf("Op = %v, want HeaderOpSet", op.Op)
	}
	if op.HeaderID != snapshot.HeaderContentType {
		t.Errorf("HeaderID = %v, want %v", op.HeaderID, snapshot.HeaderContentType)
	}
	if op.Value != "application/json" {
		t.Errorf("Value = %q, want %q", op.Value, "application/json")
	}
}

func TestPolicy_singlePolicyResponseAdd(t *testing.T) {
	out, err := Policies(&ir.Policies{
		Headers: map[string]ir.Headers{
			"sec": {
				Response: ir.HeadersOps{
					Add: map[string]string{"X-Content-Type-Options": "nosniff"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	plan := out.Headers[0].Response
	if len(plan.Ops) != 1 {
		t.Fatalf("expected 1 response op, got %d", len(plan.Ops))
	}
	op := plan.Ops[0]
	if op.Op != snapshot.HeaderOpAddIfAbsent {
		t.Errorf("Op = %v, want HeaderOpAddIfAbsent", op.Op)
	}
	if op.HeaderID != snapshot.HeaderXContentTypeOptions {
		t.Errorf("HeaderID = %v, want %v", op.HeaderID, snapshot.HeaderXContentTypeOptions)
	}
	if op.Value != "nosniff" {
		t.Errorf("Value = %q, want %q", op.Value, "nosniff")
	}
}

func TestPolicy_removeOpNoValue(t *testing.T) {
	out, err := Policies(&ir.Policies{
		Headers: map[string]ir.Headers{
			"strip": {
				Response: ir.HeadersOps{
					Remove: []string{"Server"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	plan := out.Headers[0].Response
	if len(plan.Ops) != 1 {
		t.Fatalf("expected 1 response op, got %d", len(plan.Ops))
	}
	op := plan.Ops[0]
	if op.Op != snapshot.HeaderOpRemove {
		t.Errorf("Op = %v, want HeaderOpRemove", op.Op)
	}
	if op.HeaderID != snapshot.HeaderServer {
		t.Errorf("HeaderID = %v, want %v", op.HeaderID, snapshot.HeaderServer)
	}
	if op.Value != "" {
		t.Errorf("remove op should have empty Value, got %q", op.Value)
	}
}

// Policies must be emitted in sorted (deterministic) name order.
func TestPolicy_multiplePoliciesDeterministicOrder(t *testing.T) {
	out, err := Policies(&ir.Policies{
		Headers: map[string]ir.Headers{
			"z-policy": {
				Response: ir.HeadersOps{Remove: []string{"Server"}},
			},
			"a-policy": {
				Request: ir.HeadersOps{
					Set: map[string]string{"Host": "example.com"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Headers) != 2 {
		t.Fatalf("expected 2 compiled header sets, got %d", len(out.Headers))
	}
	// a-policy sorts first → request Set Host.
	if len(out.Headers[0].Request.Ops) != 1 || out.Headers[0].Request.Ops[0].HeaderID != snapshot.HeaderHost {
		t.Errorf("first entry (a-policy) should have request Set Host, got %+v", out.Headers[0].Request.Ops)
	}
	// z-policy sorts second → response Remove Server.
	if len(out.Headers[1].Response.Ops) != 1 || out.Headers[1].Response.Ops[0].HeaderID != snapshot.HeaderServer {
		t.Errorf("second entry (z-policy) should have response Remove Server, got %+v", out.Headers[1].Response.Ops)
	}
}

func TestPolicy_unknownHeaderReturnsWrappedError(t *testing.T) {
	_, err := Policies(&ir.Policies{
		Headers: map[string]ir.Headers{
			"bad-policy": {
				Request: ir.HeadersOps{
					Set: map[string]string{"X-Not-Supported": "val"},
				},
			},
		},
	})
	if err == nil {
		t.Fatal("expected error for unsupported header")
	}
	if !strings.Contains(err.Error(), `"bad-policy"`) {
		t.Errorf("error should mention policy name: %v", err)
	}
	if !strings.Contains(err.Error(), "unsupported header") {
		t.Errorf("error should mention unsupported header: %v", err)
	}
}

// ─────────────────────── resolveHeaderID ──────────────────────────────────

func TestResolveHeaderID_knownHeaders(t *testing.T) {
	tests := []struct {
		name string
		want snapshot.HeaderID
	}{
		{"Host", snapshot.HeaderHost},
		{"Content-Type", snapshot.HeaderContentType},
		{"Content-Length", snapshot.HeaderContentLength},
		{"Authorization", snapshot.HeaderAuthorization},
		{"X-Forwarded-For", snapshot.HeaderXForwardedFor},
		{"X-Forwarded-Proto", snapshot.HeaderXForwardedProto},
		{"X-Request-ID", snapshot.HeaderXRequestID},
		{"X-Request-Id", snapshot.HeaderXRequestID}, // alternate casing alias
		{"Server", snapshot.HeaderServer},
		{"X-Content-Type-Options", snapshot.HeaderXContentTypeOptions},
		{"X-Frame-Options", snapshot.HeaderXFrameOptions},
		{"X-XSS-Protection", snapshot.HeaderXXSSProtection},
		{"X-Xss-Protection", snapshot.HeaderXXSSProtection}, // alternate casing alias
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveHeaderID(tt.name)
			if err != nil {
				t.Fatalf("resolveHeaderID(%q): unexpected error: %v", tt.name, err)
			}
			if got != tt.want {
				t.Errorf("resolveHeaderID(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestResolveHeaderID_unknownReturnsHeaderUnknownAndError(t *testing.T) {
	got, err := resolveHeaderID("X-Custom-Nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown header name")
	}
	if got != snapshot.HeaderUnknown {
		t.Errorf("got HeaderID %v, want HeaderUnknown", got)
	}
	if !strings.Contains(err.Error(), "unsupported header") {
		t.Errorf("error should mention unsupported header: %v", err)
	}
}

// ──────────────────── compileHeaderOps ────────────────────────────────────

func TestCompileHeaderOps_nilReturnsNil(t *testing.T) {
	ops, err := compileHeaderOps(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ops != nil {
		t.Fatalf("expected nil slice, got %v", ops)
	}
}

func TestCompileHeaderOps_emptyReturnsNil(t *testing.T) {
	ops, err := compileHeaderOps(&ir.HeadersOps{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ops != nil {
		t.Fatalf("expected nil slice for empty ops, got %v", ops)
	}
}

func TestCompileHeaderOps_removeOnly(t *testing.T) {
	ops, err := compileHeaderOps(&ir.HeadersOps{
		Remove: []string{"Server"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Op != snapshot.HeaderOpRemove {
		t.Errorf("Op = %v, want HeaderOpRemove", ops[0].Op)
	}
	if ops[0].HeaderID != snapshot.HeaderServer {
		t.Errorf("HeaderID = %v, want %v", ops[0].HeaderID, snapshot.HeaderServer)
	}
}

func TestCompileHeaderOps_setCarriesValue(t *testing.T) {
	ops, err := compileHeaderOps(&ir.HeadersOps{
		Set: map[string]string{"Content-Type": "text/plain"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]
	if op.Op != snapshot.HeaderOpSet {
		t.Errorf("Op = %v, want HeaderOpSet", op.Op)
	}
	if op.HeaderID != snapshot.HeaderContentType {
		t.Errorf("HeaderID = %v, want %v", op.HeaderID, snapshot.HeaderContentType)
	}
	if op.Value != "text/plain" {
		t.Errorf("Value = %q, want %q", op.Value, "text/plain")
	}
}

func TestCompileHeaderOps_addCarriesValue(t *testing.T) {
	ops, err := compileHeaderOps(&ir.HeadersOps{
		Add: map[string]string{"X-Frame-Options": "DENY"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]
	if op.Op != snapshot.HeaderOpAddIfAbsent {
		t.Errorf("Op = %v, want HeaderOpAddIfAbsent", op.Op)
	}
	if op.HeaderID != snapshot.HeaderXFrameOptions {
		t.Errorf("HeaderID = %v, want %v", op.HeaderID, snapshot.HeaderXFrameOptions)
	}
	if op.Value != "DENY" {
		t.Errorf("Value = %q, want %q", op.Value, "DENY")
	}
}

// Operations must be emitted in Remove → Set → AddIfAbsent order.
func TestCompileHeaderOps_executionOrder(t *testing.T) {
	ops, err := compileHeaderOps(&ir.HeadersOps{
		Remove: []string{"Server"},
		Set:    map[string]string{"Content-Type": "application/json"},
		Add:    map[string]string{"X-Frame-Options": "DENY"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 3 {
		t.Fatalf("expected 3 ops, got %d: %+v", len(ops), ops)
	}
	if ops[0].Op != snapshot.HeaderOpRemove {
		t.Errorf("ops[0].Op = %v, want Remove", ops[0].Op)
	}
	if ops[1].Op != snapshot.HeaderOpSet {
		t.Errorf("ops[1].Op = %v, want Set", ops[1].Op)
	}
	if ops[2].Op != snapshot.HeaderOpAddIfAbsent {
		t.Errorf("ops[2].Op = %v, want AddIfAbsent", ops[2].Op)
	}
}

func TestCompileHeaderOps_unknownHeaderInRemove(t *testing.T) {
	_, err := compileHeaderOps(&ir.HeadersOps{
		Remove: []string{"X-Not-Supported"},
	})
	if err == nil {
		t.Fatal("expected error for unsupported header in remove")
	}
	if !strings.Contains(err.Error(), "unsupported header") {
		t.Errorf("error should mention unsupported header: %v", err)
	}
}

func TestCompileHeaderOps_unknownHeaderInSet(t *testing.T) {
	_, err := compileHeaderOps(&ir.HeadersOps{
		Set: map[string]string{"X-Not-Supported": "value"},
	})
	if err == nil {
		t.Fatal("expected error for unsupported header in set")
	}
}

func TestCompileHeaderOps_unknownHeaderInAdd(t *testing.T) {
	_, err := compileHeaderOps(&ir.HeadersOps{
		Add: map[string]string{"X-Not-Supported": "value"},
	})
	if err == nil {
		t.Fatal("expected error for unsupported header in add")
	}
}

// ─────────────────── sortedStringMapKeys ──────────────────────────────────

func TestSortedStringMapKeys(t *testing.T) {
	t.Run("nil map returns nil", func(t *testing.T) {
		if got := sortedStringMapKeys(nil); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("empty map returns nil", func(t *testing.T) {
		if got := sortedStringMapKeys(map[string]string{}); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("single entry", func(t *testing.T) {
		want := []string{"Authorization"}
		if got := sortedStringMapKeys(map[string]string{"Authorization": "v"}); !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("multiple entries are lexicographically sorted", func(t *testing.T) {
		m := map[string]string{
			"Content-Type":  "a",
			"Authorization": "b",
			"Host":          "c",
		}
		want := []string{"Authorization", "Content-Type", "Host"}
		if got := sortedStringMapKeys(m); !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}
