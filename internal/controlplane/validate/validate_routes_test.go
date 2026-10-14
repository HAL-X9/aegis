package validate

import (
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

// testServices is the service table every route fixture refers to.
func testServices() schema.Services {
	return schema.Services{
		"api": {Upstream: schema.Upstream{Scheme: "http", Host: "localhost", Port: 8080}},
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	t.Run("nil config", func(t *testing.T) {
		t.Parallel()

		_, err := Validate(nil)

		assertErrorContains(t, err, "gateway configuration is nil")
	})

	t.Run("nil routes slice", func(t *testing.T) {
		t.Parallel()

		_, err := Validate(&schema.GatewayConfig{Services: testServices(), Routes: nil})

		assertErrorContains(t, err,
			"gateway config validation failed: routes are invalid",
			"routes must be provided: got nil slice",
		)
	})

	t.Run("empty routes slice is valid", func(t *testing.T) {
		t.Parallel()

		_, err := Validate(&schema.GatewayConfig{
			Services: testServices(),
			Routes:   []schema.Route{},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("minimal valid route", func(t *testing.T) {
		t.Parallel()

		cfg := &schema.GatewayConfig{
			Services: testServices(),
			Routes: []schema.Route{
				{
					Name:    "api",
					Service: "api",
					Match:   schema.Match{PathPrefix: "/api"},
				},
			},
		}

		if _, err := Validate(cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid route with methods", func(t *testing.T) {
		t.Parallel()

		cfg := &schema.GatewayConfig{
			Services: testServices(),
			Routes: []schema.Route{
				{
					Name:    "full",
					Service: "api",
					Match: schema.Match{
						PathPrefix: "/v1/",
						Methods: []string{
							"GET",
							"POST",
							"PUT",
							"DELETE",
							"PATCH",
							"OPTIONS",
							"HEAD",
						},
					},
				},
			},
		}

		if _, err := Validate(cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid route index in error", func(t *testing.T) {
		t.Parallel()

		cfg := &schema.GatewayConfig{
			Services: testServices(),
			Routes: []schema.Route{
				{
					Name:  "ok",
					Match: schema.Match{PathPrefix: "/a"},
				},
				{
					Name:  "",
					Match: schema.Match{PathPrefix: "/b"},
				},
			},
		}

		_, err := Validate(cfg)

		assertErrorContains(t, err,
			"gateway config validation failed: routes are invalid",
			"invalid route at index 1",
			"name is required",
		)
	})
}

func TestValidate_routeFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		route      schema.Route
		wantSubstr []string
	}{
		{
			name: "blank name",
			route: schema.Route{
				Name:  "   ",
				Match: schema.Match{PathPrefix: "/x"},
			},
			wantSubstr: []string{
				"name is required and must not be blank",
			},
		},
		{
			name: "blank path_prefix",
			route: schema.Route{
				Name:  "r",
				Match: schema.Match{PathPrefix: ""},
			},
			wantSubstr: []string{
				"match.path_prefix is required and must not be blank",
			},
		},
		{
			name: "path_prefix whitespace only",
			route: schema.Route{
				Name:  "r",
				Match: schema.Match{PathPrefix: "  \t "},
			},
			wantSubstr: []string{
				"match.path_prefix is required and must not be blank",
			},
		},
		{
			name: "path_prefix without leading slash",
			route: schema.Route{
				Name:  "r",
				Match: schema.Match{PathPrefix: "api/"},
			},
			wantSubstr: []string{
				"match.path_prefix must start with '/'",
			},
		},
		{
			name: "unsupported method",
			route: schema.Route{
				Name: "r",
				Match: schema.Match{
					PathPrefix: "/",
					Methods:    []string{"GET", "TRACE"},
				},
			},
			wantSubstr: []string{
				"unsupported method",
				"TRACE",
			},
		},
		{
			name: "empty header key",
			route: schema.Route{
				Name: "r",
				Match: schema.Match{
					PathPrefix: "/",
					Headers: map[string][]string{
						"": {"v"},
					},
				},
			},
			wantSubstr: []string{
				"invalid header key: empty string",
			},
		},
		{
			name: "header key exceeds max length",
			route: schema.Route{
				Name: "r",
				Match: schema.Match{
					PathPrefix: "/",
					Headers: map[string][]string{
						strings.Repeat("a", 257): {"v"},
					},
				},
			},
			wantSubstr: []string{
				"exceeds 256 characters",
			},
		},
		{
			name: "empty allowed header value",
			route: schema.Route{
				Name:    "r",
				Service: "api",
				Match: schema.Match{
					PathPrefix: "/",
					Headers: map[string][]string{
						"Authorization": {"Bearer token", ""},
					},
				},
			},
			wantSubstr: []string{
				`invalid header key "Authorization": contains an empty allowed value`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Validate(&schema.GatewayConfig{
				Services: testServices(),
				Routes:   []schema.Route{tt.route},
			})

			assertErrorContains(t, err, append([]string{
				"gateway config validation failed: routes are invalid",
				"invalid route at index 0",
			}, tt.wantSubstr...)...)
		})
	}
}

func TestValidateRoutes(t *testing.T) {
	t.Parallel()

	t.Run("nil slice", func(t *testing.T) {
		t.Parallel()

		err := validateRoutes(nil)

		assertErrorContains(t, err,
			"routes must be provided: got nil slice",
		)
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()

		if err := validateRoutes([]schema.Route{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid single route", func(t *testing.T) {
		t.Parallel()

		routes := []schema.Route{
			{
				Name:  "a",
				Match: schema.Match{PathPrefix: "/"},
			},
		}

		if err := validateRoutes(routes); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidateRoute(t *testing.T) {
	t.Parallel()

	t.Run("nil route", func(t *testing.T) {
		t.Parallel()

		err := validateRoute(nil)

		assertErrorContains(t, err,
			"route validation failed: route is nil",
		)
	})

	t.Run("valid minimal", func(t *testing.T) {
		t.Parallel()

		r := &schema.Route{
			Name:  "x",
			Match: schema.Match{PathPrefix: "/p"},
		}

		if err := validateRoute(r); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidate_errorWrapping(t *testing.T) {
	t.Parallel()

	_, err := Validate(&schema.GatewayConfig{
		Services: testServices(),
		Routes: []schema.Route{
			{
				Name: "bad",
				Match: schema.Match{
					PathPrefix: "",
				},
			},
		},
	})

	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(
		err.Error(),
		"gateway config validation failed: routes are invalid",
	) {
		t.Fatalf("missing outer context: %v", err)
	}

	if !strings.Contains(
		err.Error(),
		"match.path_prefix is required and must not be blank",
	) {
		t.Fatalf("missing inner detail: %v", err)
	}
}

func assertErrorContains(t *testing.T, err error, substr ...string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected non-nil error")
	}

	msg := err.Error()

	for _, s := range substr {
		if !strings.Contains(msg, s) {
			t.Fatalf(
				"error\n%q\ndoes not contain %q",
				msg,
				s,
			)
		}
	}
}

func TestValidatePathPrefix(t *testing.T) {
	t.Parallel()

	valid := []string{
		"/",
		"/api",
		"/api/",
		"/api/v1/users",
		"/users/:id",
		"/users/:user_id/orders/:order_id",
		"/files/*",
		"/files/*path",
		"/v1/items:batchGet",
	}
	for _, path := range valid {
		if err := validatePathPrefix(path); err != nil {
			t.Errorf("validatePathPrefix(%q) = %v, want nil", path, err)
		}
	}

	invalid := []struct {
		path       string
		wantSubstr string
	}{
		{path: "", wantSubstr: "is required and must not be blank"},
		{path: "api", wantSubstr: "must start with '/'"},
		{path: "/api?x=1", wantSubstr: "must not contain '?' or '#'"},
		{path: "/api#top", wantSubstr: "must not contain '?' or '#'"},
		{path: "//", wantSubstr: "has an empty segment"},
		{path: "/api//v1", wantSubstr: "has an empty segment"},
		{path: "/api//", wantSubstr: "has an empty segment"},
		{path: "/api/./v1", wantSubstr: `has a dot segment "."`},
		{path: "/api/../admin", wantSubstr: `has a dot segment ".."`},
		{path: "/users/:", wantSubstr: "has an invalid parameter"},
		{path: "/users/:user-id", wantSubstr: "has an invalid parameter"},
		{path: "/files/*/meta", wantSubstr: "is not the last segment"},
		{path: "/files/*file.name", wantSubstr: "has an invalid wildcard"},
	}
	for _, tt := range invalid {
		assertErrorContains(t, validatePathPrefix(tt.path), tt.wantSubstr)
	}
}
