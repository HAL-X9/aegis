package compiler

import (
	"reflect"
	"strings"
	"testing"

	"github.com/aegis/internal/contracts/methodmask"
	"github.com/aegis/internal/controlplane/model"
)

func TestCompile(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		_, err := Compile(nil)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "manifest is nil") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty routes", func(t *testing.T) {
		out, err := Compile(&model.GatewayConfig{})
		if err != nil {
			t.Fatal(err)
		}
		if out == nil || len(out.Routes) != 0 {
			t.Fatalf("got %#v", out)
		}
	})

	t.Run("single route", func(t *testing.T) {
		cfg := &model.GatewayConfig{
			Routes: []model.Route{
				{
					Name: "api",
					Match: model.Match{
						PathPrefix: "/v1/",
						Methods:    []string{"GET", "POST"},
					},
					Upstream: model.Upstream{
						Scheme: "https",
						Host:   "api.example.com",
						Port:   443,
					},
				},
			},
		}
		out, err := Compile(cfg)
		if err != nil {
			t.Fatal(err)
		}
		methodMask, err := methodmask.BuildMethodMask([]string{"GET", "POST"})
		if err != nil {
			t.Fatal(err)
		}
		want := CompiledGatewayConfig{
			Routes: []CompiledRoute{
				{
					Name: "api",
					Match: CompiledMatch{
						PathPrefix: "/v1/",
						Methods:    methodMask,
					},
					Upstream: "https://api.example.com:443",
				},
			},
		}
		if !reflect.DeepEqual(*out, want) {
			t.Errorf("diff (-got +want)\ngot:  %+v\nwant: %+v", *out, want)
		}
	})

	t.Run("empty methods list is MethodAll", func(t *testing.T) {
		cfg := &model.GatewayConfig{
			Routes: []model.Route{
				{
					Name:  "any",
					Match: model.Match{PathPrefix: "/", Methods: nil},
					Upstream: model.Upstream{
						Scheme: "http",
						Host:   "127.0.0.1",
						Port:   80,
					},
				},
			},
		}
		out, err := Compile(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if got := out.Routes[0].Match.Methods; got != methodmask.MethodAll {
			t.Fatalf("Methods = %#v, want MethodAll (%#v)", got, methodmask.MethodAll)
		}
	})

	t.Run("invalid HTTP method", func(t *testing.T) {
		cfg := &model.GatewayConfig{
			Routes: []model.Route{
				{
					Name: "bad",
					Match: model.Match{
						PathPrefix: "/",
						Methods:    []string{"BOGUS"},
					},
					Upstream: model.Upstream{Scheme: "http", Host: "h", Port: 1},
				},
			},
		}
		_, err := Compile(cfg)
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
		cfg := &model.GatewayConfig{
			Routes: []model.Route{
				{Name: "first", Match: model.Match{PathPrefix: "/a"}, Upstream: model.Upstream{Scheme: "http", Host: "a", Port: 1}},
				{Name: "second", Match: model.Match{PathPrefix: "/b"}, Upstream: model.Upstream{Scheme: "http", Host: "b", Port: 2}},
			},
		}
		out, err := Compile(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Routes) != 2 || out.Routes[0].Name != "first" || out.Routes[1].Name != "second" {
			t.Fatalf("routes: %#v", out.Routes)
		}
	})
}

func TestBuildUpstreamOriginURL(t *testing.T) {
	tests := []struct {
		name  string
		route model.Route
		want  string
	}{
		{
			name: "https hostname",
			route: model.Route{
				Upstream: model.Upstream{Scheme: "https", Host: "example.com", Port: 443},
			},
			want: "https://example.com:443",
		},
		{
			name: "http ipv4",
			route: model.Route{
				Upstream: model.Upstream{Scheme: "http", Host: "10.0.0.1", Port: 8080},
			},
			want: "http://10.0.0.1:8080",
		},
		{
			name: "ipv6 literal host",
			route: model.Route{
				Upstream: model.Upstream{Scheme: "http", Host: "2001:db8::1", Port: 80},
			},
			want: "http://[2001:db8::1]:80",
		},
		{
			name: "port zero",
			route: model.Route{
				Upstream: model.Upstream{Scheme: "http", Host: "x", Port: 0},
			},
			want: "http://x:0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildUpstreamOriginURL(tt.route); got != tt.want {
				t.Errorf("BuildUpstreamOriginURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
