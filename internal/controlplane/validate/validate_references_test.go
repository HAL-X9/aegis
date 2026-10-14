package validate

import (
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

func referencesConfig(routes []schema.Route, policies schema.Policies) *schema.GatewayConfig {
	return &schema.GatewayConfig{
		Services: testServices(),
		Policies: policies,
		Routes:   routes,
	}
}

func route(policies ...string) schema.Route {
	refs := make([]schema.Policy, 0, len(policies))
	for _, name := range policies {
		refs = append(refs, schema.Policy{Name: name})
	}

	return schema.Route{
		Name:     "r",
		Service:  "api",
		Match:    schema.Match{PathPrefix: "/"},
		Policies: refs,
	}
}

func TestValidate_servicesRequired(t *testing.T) {
	t.Parallel()

	_, err := Validate(&schema.GatewayConfig{})

	assertErrorContains(t, err, "services must be provided: got nil map")
}

func TestValidate_references(t *testing.T) {
	t.Parallel()

	headers := func(request schema.HeadersOps) schema.Headers {
		return schema.Headers{Request: request}
	}

	tests := []struct {
		name     string
		routes   []schema.Route
		policies schema.Policies
		wantErr  []string
	}{
		{
			name:   "unknown service",
			routes: []schema.Route{{Name: "r", Service: "ghost", Match: schema.Match{PathPrefix: "/"}}},
			wantErr: []string{
				`service "ghost" is not defined`,
			},
		},
		{
			name:   "unknown policy",
			routes: []schema.Route{route("missing")},
			wantErr: []string{
				`references unknown policy "missing"`,
			},
		},
		{
			name:   "multiple rate-limit policies",
			routes: []schema.Route{route("rl-a", "rl-b")},
			policies: schema.Policies{
				RateLimits: map[string]schema.RateLimit{
					"rl-a": {Rate: 10, Burst: 5},
					"rl-b": {Rate: 20, Burst: 5},
				},
			},
			wantErr: []string{
				"references multiple rate-limit policies",
			},
		},
		{
			name:   "one rate-limit policy is valid",
			routes: []schema.Route{route("rl")},
			policies: schema.Policies{
				RateLimits: map[string]schema.RateLimit{"rl": {Rate: 10, Burst: 5}},
			},
		},
		{
			name:   "header changed by two policies",
			routes: []schema.Route{route("a", "b")},
			policies: schema.Policies{
				Headers: map[string]schema.Headers{
					"a": headers(schema.HeadersOps{Set: map[string]string{"X-Frame-Options": "DENY"}}),
					"b": headers(schema.HeadersOps{Remove: []string{"x-frame-options"}}),
				},
			},
			wantErr: []string{
				`header "x-frame-options" is changed by both policy "a" and policy "b"`,
			},
		},
		{
			name:   "same policy referenced twice",
			routes: []schema.Route{route("a", "a")},
			policies: schema.Policies{
				Headers: map[string]schema.Headers{
					"a": headers(schema.HeadersOps{Add: map[string]string{"X-Trace": "1"}}),
				},
			},
			wantErr: []string{
				`header "X-Trace" is changed more than once by policy "a"`,
			},
		},
		{
			name:   "policies touching different headers are valid",
			routes: []schema.Route{route("a", "b")},
			policies: schema.Policies{
				Headers: map[string]schema.Headers{
					"a": headers(schema.HeadersOps{Add: map[string]string{"X-A": "1"}}),
					"b": headers(schema.HeadersOps{Add: map[string]string{"X-B": "1"}}),
				},
			},
		},
		{
			name:   "same header in different directions is valid",
			routes: []schema.Route{route("a", "b")},
			policies: schema.Policies{
				Headers: map[string]schema.Headers{
					"a": {Request: schema.HeadersOps{Set: map[string]string{"X-Id": "1"}}},
					"b": {Response: schema.HeadersOps{Set: map[string]string{"X-Id": "1"}}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Validate(referencesConfig(tt.routes, tt.policies))

			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			assertErrorContains(t, err, tt.wantErr...)
		})
	}
}
