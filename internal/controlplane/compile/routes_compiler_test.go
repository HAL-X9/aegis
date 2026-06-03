package compile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aegis/internal/contracts/methodmask"
	"github.com/aegis/internal/controlplane/ir"
	"github.com/aegis/internal/controlplane/snapshot"
)

func TestRoutes(t *testing.T) {
	t.Run("empty routes", func(t *testing.T) {
		out, err := Routes(nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 0 {
			t.Fatalf("got %#v", out)
		}
	})

	t.Run("single route", func(t *testing.T) {
		routes := []ir.Route{
			{
				Name: "api",
				Match: ir.Match{
					PathPrefix: "/v1/",
					Methods:    []string{"GET", "POST"},
				},
				Upstream: ir.Upstream{
					Scheme: "https",
					Host:   "api.example.com",
					Port:   443,
				},
			},
		}

		out, err := Routes(routes, nil)
		if err != nil {
			t.Fatal(err)
		}

		methodMask, err := methodmask.BuildMethodMask([]string{"GET", "POST"})
		if err != nil {
			t.Fatal(err)
		}

		want := []snapshot.CompiledRoute{
			{
				Name: "api",
				Match: snapshot.CompiledMatch{
					PathPrefix: "/v1/",
					Methods:    methodMask,
				},
				Upstream: "https://api.example.com:443",
			},
		}

		if !reflect.DeepEqual(out, want) {
			t.Errorf("diff (-got +want)\ngot:  %+v\nwant: %+v", out, want)
		}
	})

	t.Run("empty methods list is MethodAll", func(t *testing.T) {
		routes := []ir.Route{
			{
				Name: "any",
				Match: ir.Match{
					PathPrefix: "/",
					Methods:    nil,
				},
				Upstream: ir.Upstream{
					Scheme: "http",
					Host:   "127.0.0.1",
					Port:   80,
				},
			},
		}

		policies := ir.NormalizedPolicies{
			Headers: make(map[string]ir.NormalizedHeaders),
		}

		out, err := Routes(routes, &policies)
		if err != nil {
			t.Fatal(err)
		}

		if got := out[0].Match.Methods; got != methodmask.MethodAll {
			t.Fatalf("Methods = %#v, want MethodAll (%#v)", got, methodmask.MethodAll)
		}
	})

	t.Run("invalid HTTP method", func(t *testing.T) {
		routes := []ir.Route{
			{
				Name: "bad",
				Match: ir.Match{
					PathPrefix: "/",
					Methods:    []string{"BOGUS"},
				},
				Upstream: ir.Upstream{
					Scheme: "http",
					Host:   "h",
					Port:   1,
				},
			},
		}

		policies := ir.NormalizedPolicies{
			Headers: make(map[string]ir.NormalizedHeaders),
		}

		_, err := Routes(routes, &policies)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), `route "bad"`) {
			t.Fatalf("error should mention route name: %v", err)
		}
		if !strings.Contains(err.Error(), "compile method mask") {
			t.Fatalf("error should mention method mask: %v", err)
		}
		if !strings.Contains(err.Error(), "unsupported HTTP method") {
			t.Fatalf("error should wrap methodmask error: %v", err)
		}
	})

	t.Run("route order preserved", func(t *testing.T) {
		routes := []ir.Route{
			{
				Name: "first",
				Match: ir.Match{
					PathPrefix: "/a",
				},
				Upstream: ir.Upstream{
					Scheme: "http",
					Host:   "a",
					Port:   1,
				},
			},
			{
				Name: "second",
				Match: ir.Match{
					PathPrefix: "/b",
				},
				Upstream: ir.Upstream{
					Scheme: "http",
					Host:   "b",
					Port:   2,
				},
			},
		}

		policies := ir.NormalizedPolicies{
			Headers: make(map[string]ir.NormalizedHeaders),
		}

		out, err := Routes(routes, &policies)
		if err != nil {
			t.Fatal(err)
		}

		if len(out) != 2 || out[0].Name != "first" || out[1].Name != "second" {
			t.Fatalf("routes: %#v", out)
		}
	})
}

func TestUpstreamOriginURL(t *testing.T) {
	tests := []struct {
		name     string
		upstream ir.Upstream
		want     string
	}{
		{
			name: "https hostname",
			upstream: ir.Upstream{
				Scheme: "https",
				Host:   "example.com",
				Port:   443,
			},
			want: "https://example.com:443",
		},
		{
			name: "http ipv4",
			upstream: ir.Upstream{
				Scheme: "http",
				Host:   "10.0.0.1",
				Port:   8080,
			},
			want: "http://10.0.0.1:8080",
		},
		{
			name: "ipv6 literal host",
			upstream: ir.Upstream{
				Scheme: "http",
				Host:   "2001:db8::1",
				Port:   80,
			},
			want: "http://[2001:db8::1]:80",
		},
		{
			name: "port zero",
			upstream: ir.Upstream{
				Scheme: "http",
				Host:   "x",
				Port:   0,
			},
			want: "http://x:0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := upstreamOriginURL(tt.upstream); got != tt.want {
				t.Errorf("upstreamOriginURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
