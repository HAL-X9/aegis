package validate

import (
	"strconv"
	"strings"
	"testing"

	"github.com/aegis/internal/controlplane/model"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	t.Run("nil config", func(t *testing.T) {
		t.Parallel()
		err := Validate(nil)
		assertErrorContains(t, err, "gateway configuration is nil")
	})

	t.Run("nil routes slice", func(t *testing.T) {
		t.Parallel()
		err := Validate(&model.GatewayConfig{Routes: nil})
		assertErrorContains(t, err,
			"gateway config validation failed: routes are invalid",
			"routes must be provided: got nil slice",
		)
	})

	t.Run("empty routes slice is valid", func(t *testing.T) {
		t.Parallel()
		err := Validate(&model.GatewayConfig{Routes: []model.Route{}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("minimal valid route", func(t *testing.T) {
		t.Parallel()
		cfg := &model.GatewayConfig{
			Routes: []model.Route{
				{
					Name:     "api",
					Match:    model.Match{PathPrefix: "/api"},
					Upstream: model.Upstream{Scheme: "http", Host: "127.0.0.1", Port: 8080},
				},
			},
		}
		if err := Validate(cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid route with methods and https", func(t *testing.T) {
		t.Parallel()
		cfg := &model.GatewayConfig{
			Routes: []model.Route{
				{
					Name: "full",
					Match: model.Match{
						PathPrefix: "/v1/",
						Methods:    []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"},
					},
					Upstream: model.Upstream{Scheme: "https", Host: "example.com", Port: 443},
				},
			},
		}
		if err := Validate(cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid route index in error", func(t *testing.T) {
		t.Parallel()
		cfg := &model.GatewayConfig{
			Routes: []model.Route{
				{
					Name:     "ok",
					Match:    model.Match{PathPrefix: "/a"},
					Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 1},
				},
				{
					Name:     "",
					Match:    model.Match{PathPrefix: "/b"},
					Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 2},
				},
			},
		}
		err := Validate(cfg)
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
		route      model.Route
		wantSubstr []string
	}{
		{
			name: "blank name",
			route: model.Route{
				Name:     "   ",
				Match:    model.Match{PathPrefix: "/x"},
				Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 1},
			},
			wantSubstr: []string{"name is required and must not be blank"},
		},
		{
			name: "blank path_prefix",
			route: model.Route{
				Name:     "r",
				Match:    model.Match{PathPrefix: ""},
				Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 1},
			},
			wantSubstr: []string{"match.path_prefix is required and must not be blank"},
		},
		{
			name: "path_prefix whitespace only",
			route: model.Route{
				Name:     "r",
				Match:    model.Match{PathPrefix: "  \t "},
				Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 1},
			},
			wantSubstr: []string{"match.path_prefix is required and must not be blank"},
		},
		{
			name: "path_prefix without leading slash",
			route: model.Route{
				Name:     "r",
				Match:    model.Match{PathPrefix: "api/"},
				Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 1},
			},
			wantSubstr: []string{"match.path_prefix must start with '/'"},
		},
		{
			name: "unsupported method",
			route: model.Route{
				Name:     "r",
				Match:    model.Match{PathPrefix: "/", Methods: []string{"GET", "TRACE"}},
				Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 1},
			},
			wantSubstr: []string{"unsupported method", "TRACE"},
		},
		{
			name: "empty upstream scheme",
			route: model.Route{
				Name:     "r",
				Match:    model.Match{PathPrefix: "/"},
				Upstream: model.Upstream{Scheme: "", Host: "h", Port: 1},
			},
			wantSubstr: []string{"upstream.scheme is required and must be non-empty"},
		},
		{
			name: "upstream scheme not http or https",
			route: model.Route{
				Name:     "r",
				Match:    model.Match{PathPrefix: "/"},
				Upstream: model.Upstream{Scheme: "grpc", Host: "h", Port: 1},
			},
			wantSubstr: []string{"upstream.scheme must be one of: http, https"},
		},
		{
			name: "blank upstream host",
			route: model.Route{
				Name:     "r",
				Match:    model.Match{PathPrefix: "/"},
				Upstream: model.Upstream{Scheme: "http", Host: "  ", Port: 1},
			},
			wantSubstr: []string{"upstream.host is required and must not be blank"},
		},
		{
			name: "port zero",
			route: model.Route{
				Name:     "r",
				Match:    model.Match{PathPrefix: "/"},
				Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 0},
			},
			wantSubstr: []string{"upstream.port must be in range 1..65535"},
		},
		{
			name: "port negative",
			route: model.Route{
				Name:     "r",
				Match:    model.Match{PathPrefix: "/"},
				Upstream: model.Upstream{Scheme: "http", Host: "h", Port: -1},
			},
			wantSubstr: []string{"upstream.port must be in range 1..65535"},
		},
		{
			name: "port above 65535",
			route: model.Route{
				Name:     "r",
				Match:    model.Match{PathPrefix: "/"},
				Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 65536},
			},
			wantSubstr: []string{"upstream.port must be in range 1..65535"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := Validate(&model.GatewayConfig{Routes: []model.Route{tt.route}})
			assertErrorContains(t, err, append([]string{
				"gateway config validation failed: routes are invalid",
				"invalid route at index 0",
			}, tt.wantSubstr...)...)
		})
	}
}

func TestValidate_acceptedPortBoundaries(t *testing.T) {
	t.Parallel()
	for _, port := range []int{1, 65535} {
		port := port
		t.Run("port_"+strconv.Itoa(port), func(t *testing.T) {
			t.Parallel()
			cfg := &model.GatewayConfig{
				Routes: []model.Route{
					{
						Name:     "edge",
						Match:    model.Match{PathPrefix: "/"},
						Upstream: model.Upstream{Scheme: "http", Host: "h", Port: port},
					},
				},
			}
			if err := Validate(cfg); err != nil {
				t.Fatalf("port %d: unexpected error: %v", port, err)
			}
		})
	}
}

func TestValidateRoutes(t *testing.T) {
	t.Parallel()

	t.Run("nil slice", func(t *testing.T) {
		t.Parallel()
		err := validateRoutes(nil)
		assertErrorContains(t, err, "routes must be provided: got nil slice")
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()
		if err := validateRoutes([]model.Route{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid single route", func(t *testing.T) {
		t.Parallel()
		routes := []model.Route{
			{
				Name:     "a",
				Match:    model.Match{PathPrefix: "/"},
				Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 1},
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
		assertErrorContains(t, err, "route validation failed: route is nil")
	})

	t.Run("valid minimal", func(t *testing.T) {
		t.Parallel()
		r := &model.Route{
			Name:     "x",
			Match:    model.Match{PathPrefix: "/p"},
			Upstream: model.Upstream{Scheme: "https", Host: "h", Port: 443},
		}
		if err := validateRoute(r); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestValidate_errorWrapping(t *testing.T) {
	t.Parallel()
	err := Validate(&model.GatewayConfig{
		Routes: []model.Route{
			{Name: "bad", Match: model.Match{PathPrefix: "/"}, Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 0}},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	// Outermost message from Validate; inner from validateRoute chain.
	if !strings.Contains(err.Error(), "gateway config validation failed: routes are invalid") {
		t.Fatalf("missing outer context: %v", err)
	}
	if !strings.Contains(err.Error(), "upstream.port must be in range") {
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
			t.Fatalf("error\n%q\ndoes not contain %q", msg, s)
		}
	}
}
