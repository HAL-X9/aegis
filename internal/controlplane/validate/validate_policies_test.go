package validate

import (
	"testing"

	"github.com/aegis/internal/controlplane/schema"
)

// ─────────────────────── validatePolicies ─────────────────────────────────

func TestValidatePolicies_nilReturnsError(t *testing.T) {
	t.Parallel()
	assertErrorContains(t, validatePolicies(nil), "policies configuration must not be nil")
}

func TestValidatePolicies_emptyHeadersIsValid(t *testing.T) {
	t.Parallel()
	if err := validatePolicies(&schema.Policies{Headers: map[string]schema.Headers{}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePolicies_nilHeadersMapIsValid(t *testing.T) {
	t.Parallel()
	if err := validatePolicies(&schema.Policies{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePolicies_validPolicyAllOps(t *testing.T) {
	t.Parallel()
	policy := &schema.Policies{
		Headers: map[string]schema.Headers{
			"security": {
				Request: schema.HeadersOps{
					Set:    map[string]string{"Content-Type": "application/json"},
					Add:    map[string]string{"X-Request-ID": "generated"},
					Remove: []string{"Authorization"},
				},
				Response: schema.HeadersOps{
					Add:    map[string]string{"X-Frame-Options": "DENY"},
					Remove: []string{"Server"},
				},
			},
		},
	}
	if err := validatePolicies(policy); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePolicies_invalidHeaderInAdd(t *testing.T) {
	t.Parallel()
	policy := &schema.Policies{
		Headers: map[string]schema.Headers{
			"my-policy": {
				Request: schema.HeadersOps{
					Add: map[string]string{"invalid header": "val"},
				},
			},
		},
	}
	err := validatePolicies(policy)
	assertErrorContains(t, err,
		`"my-policy"`,
		"invalid header name in add operation",
		`"invalid header"`,
	)
}

func TestValidatePolicies_invalidHeaderInSet(t *testing.T) {
	t.Parallel()
	policy := &schema.Policies{
		Headers: map[string]schema.Headers{
			"my-policy": {
				Request: schema.HeadersOps{
					Set: map[string]string{"bad\theader": "val"},
				},
			},
		},
	}
	err := validatePolicies(policy)
	assertErrorContains(t, err,
		`"my-policy"`,
		"invalid header name in set operation",
	)
}

func TestValidatePolicies_invalidHeaderInRemove(t *testing.T) {
	t.Parallel()
	policy := &schema.Policies{
		Headers: map[string]schema.Headers{
			"strip-policy": {
				Response: schema.HeadersOps{
					Remove: []string{"bad header name"},
				},
			},
		},
	}
	err := validatePolicies(policy)
	assertErrorContains(t, err,
		`"strip-policy"`,
		"invalid header name in remove operation",
	)
}

func TestValidatePolicies_errorMentionsPolicyName(t *testing.T) {
	t.Parallel()
	policy := &schema.Policies{
		Headers: map[string]schema.Headers{
			"alpha": {
				Request: schema.HeadersOps{
					Remove: []string{""},
				},
			},
		},
	}
	assertErrorContains(t, validatePolicies(policy), `"alpha"`)
}

func TestValidatePolicies_emptyPolicyNameRejected(t *testing.T) {
	t.Parallel()
	policy := &schema.Policies{
		Headers: map[string]schema.Headers{
			"": {
				Response: schema.HeadersOps{
					Add: map[string]string{"X-Frame-Options": "DENY"},
				},
			},
		},
	}
	assertErrorContains(t, validatePolicies(policy), "policy name", "must not be empty")
}

func TestValidatePolicies_conflictReportsPolicyAndDirection(t *testing.T) {
	t.Parallel()
	policy := &schema.Policies{
		Headers: map[string]schema.Headers{
			"security": {
				Response: schema.HeadersOps{
					Set:    map[string]string{"X-Frame-Options": "DENY"},
					Remove: []string{"X-Frame-Options"},
				},
			},
		},
	}
	assertErrorContains(t, validatePolicies(policy),
		`"security"`,
		"response",
		`"X-Frame-Options"`,
	)
}

// ────────────────── validateHeadersOps: op conflicts ──────────────────────

func TestValidateHeadersOps_conflictAddAndSet(t *testing.T) {
	t.Parallel()
	ops := &schema.HeadersOps{
		Add: map[string]string{"X-Token": "a"},
		Set: map[string]string{"X-Token": "b"},
	}
	assertErrorContains(t, validateHeadersOps("request", ops),
		`"X-Token"`,
		`"add"`,
		`"set"`,
	)
}

func TestValidateHeadersOps_conflictSetAndRemove(t *testing.T) {
	t.Parallel()
	ops := &schema.HeadersOps{
		Set:    map[string]string{"Server": "aegis"},
		Remove: []string{"Server"},
	}
	assertErrorContains(t, validateHeadersOps("response", ops),
		`"Server"`,
		`"set"`,
		`"remove"`,
	)
}

func TestValidateHeadersOps_conflictAddAndRemove(t *testing.T) {
	t.Parallel()
	ops := &schema.HeadersOps{
		Add:    map[string]string{"X-Request-ID": "gen"},
		Remove: []string{"X-Request-ID"},
	}
	assertErrorContains(t, validateHeadersOps("request", ops),
		`"X-Request-ID"`,
		`"add"`,
		`"remove"`,
	)
}

func TestValidateHeadersOps_conflictIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	ops := &schema.HeadersOps{
		Set:    map[string]string{"X-Frame-Options": "DENY"},
		Remove: []string{"x-frame-options"},
	}
	assertErrorContains(t, validateHeadersOps("response", ops),
		`"set"`,
		`"remove"`,
	)
}

func TestValidateHeadersOps_duplicateInRemove(t *testing.T) {
	t.Parallel()
	ops := &schema.HeadersOps{
		Remove: []string{"Server", "server"},
	}
	assertErrorContains(t, validateHeadersOps("response", ops),
		"more than once",
		`"remove"`,
	)
}

func TestValidateHeadersOps_sameHeaderAcrossDirectionsIsValid(t *testing.T) {
	t.Parallel()
	// A header may legitimately be added on the request path and removed on the
	// response path: conflicts are scoped to a single direction only.
	h := &schema.Headers{
		Request:  schema.HeadersOps{Add: map[string]string{"X-Trace": "1"}},
		Response: schema.HeadersOps{Remove: []string{"X-Trace"}},
	}
	if err := validateHeaders("cross-direction", h); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateHeadersOps_allThreeGroupsDistinctIsValid(t *testing.T) {
	t.Parallel()
	ops := &schema.HeadersOps{
		Add:    map[string]string{"X-A": "1"},
		Set:    map[string]string{"X-B": "2"},
		Remove: []string{"X-C"},
	}
	if err := validateHeadersOps("request", ops); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ─────────────────────── validateHeaders ──────────────────────────────────

func TestValidateHeaders_nilReturnsError(t *testing.T) {
	t.Parallel()
	assertErrorContains(t, validateHeaders("p", nil), "headers configuration must not be nil")
}

func TestValidateHeaders_emptyIsValid(t *testing.T) {
	t.Parallel()
	if err := validateHeaders("p", &schema.Headers{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateHeaders_invalidRequestOpsWrapsName(t *testing.T) {
	t.Parallel()
	h := &schema.Headers{
		Request: schema.HeadersOps{Add: map[string]string{"bad name": "v"}},
	}
	assertErrorContains(t, validateHeaders("my-policy", h), `"my-policy"`, "request")
}

func TestValidateHeaders_invalidResponseOpsWrapsName(t *testing.T) {
	t.Parallel()
	h := &schema.Headers{
		Response: schema.HeadersOps{Remove: []string{"bad name"}},
	}
	assertErrorContains(t, validateHeaders("my-policy", h), `"my-policy"`, "response")
}

// ────────────────────── validateHeadersOps ────────────────────────────────

func TestValidateHeadersOps_nilReturnsError(t *testing.T) {
	t.Parallel()
	assertErrorContains(t, validateHeadersOps("request", nil), "header operations configuration must not be nil")
}

func TestValidateHeadersOps_emptyIsValid(t *testing.T) {
	t.Parallel()
	if err := validateHeadersOps("request", &schema.HeadersOps{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateHeadersOps_validNames(t *testing.T) {
	t.Parallel()
	ops := &schema.HeadersOps{
		Add:    map[string]string{"X-Frame-Options": "DENY", "Content-Type": "text/plain"},
		Set:    map[string]string{"Authorization": "Bearer token"},
		Remove: []string{"Server", "X-Powered-By"},
	}
	if err := validateHeadersOps("request", ops); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateHeadersOps_emptyStringInAdd(t *testing.T) {
	t.Parallel()
	assertErrorContains(t,
		validateHeadersOps("request", &schema.HeadersOps{Add: map[string]string{"": "v"}}),
		"invalid header name in add operation",
	)
}

func TestValidateHeadersOps_emptyStringInSet(t *testing.T) {
	t.Parallel()
	assertErrorContains(t,
		validateHeadersOps("request", &schema.HeadersOps{Set: map[string]string{"": "v"}}),
		"invalid header name in set operation",
	)
}

func TestValidateHeadersOps_emptyStringInRemove(t *testing.T) {
	t.Parallel()
	assertErrorContains(t,
		validateHeadersOps("response", &schema.HeadersOps{Remove: []string{""}}),
		"invalid header name in remove operation",
	)
}

// ─────────────────────── validateHeaderName ───────────────────────────────

func TestValidateHeaderName_validNames(t *testing.T) {
	t.Parallel()

	valid := []string{
		"Content-Type",
		"Authorization",
		"X-Request-ID",
		"Accept",
		"Cache-Control",
		"X-Custom-Header",
		"X-Forwarded-For",
	}
	for _, name := range valid {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := validateHeaderName(name); err != nil {
				t.Errorf("validateHeaderName(%q): unexpected error: %v", name, err)
			}
		})
	}
}

func TestValidateHeaderName_invalidNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		headerName string
	}{
		{"empty string", ""},
		{"space in name", "invalid header"},
		{"tab in name", "bad\theader"},
		{"colon in name", "bad:header"},
		{"newline in name", "bad\nheader"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateHeaderName(tt.headerName)
			if err == nil {
				t.Fatalf("validateHeaderName(%q): expected error, got nil", tt.headerName)
			}
			assertErrorContains(t, err, "invalid header name")
		})
	}
}
