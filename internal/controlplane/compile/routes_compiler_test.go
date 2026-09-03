package compile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
)

func TestRoutes(t *testing.T) {
	t.Run("empty routes", func(t *testing.T) {
		out, err := Routes(nil, nil, nil)
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
				Name:    "api",
				Service: "api-service",
				Match: ir.Match{
					PathPrefix: "/v1/",
					Methods:    []string{"GET", "POST"},
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		out, err := Routes(serviceIDs, routes, nil)
		if err != nil {
			t.Fatal(err)
		}

		methodMask, err := methodmask.BuildMethodMask([]string{"GET", "POST"})
		if err != nil {
			t.Fatal(err)
		}

		want := []snapshot.CompiledRoute{
			{
				Name:    "api",
				Service: snapshot.ServiceID(0),
				Match: snapshot.CompiledMatch{
					PathPrefix: "/v1/",
					Methods:    methodMask,
				},
				Policies: snapshot.CompiledRoutePolicies{
					RateLimitID: -1, // no rate-limit policy referenced
				},
			},
		}

		if !reflect.DeepEqual(out, want) {
			t.Errorf("diff (-got +want)\ngot:  %+v\nwant: %+v", out, want)
		}
	})

	t.Run("unknown service", func(t *testing.T) {
		routes := []ir.Route{
			{
				Name:    "api",
				Service: "missing",
				Match: ir.Match{
					PathPrefix: "/",
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		_, err := Routes(serviceIDs, routes, nil)
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), `route "api"`) {
			t.Fatalf("error should mention route name: %v", err)
		}

		if !strings.Contains(err.Error(), `unknown service "missing"`) {
			t.Fatalf("error should mention unknown service: %v", err)
		}
	})

	t.Run("empty methods list is MethodAll", func(t *testing.T) {
		routes := []ir.Route{
			{
				Name:    "any",
				Service: "api-service",
				Match: ir.Match{
					PathPrefix: "/",
					Methods:    nil,
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		policies := ir.Policies{
			Headers: make(map[string]ir.Headers),
		}

		out, err := Routes(serviceIDs, routes, &policies)
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
				Name:    "bad",
				Service: "api-service",
				Match: ir.Match{
					PathPrefix: "/",
					Methods:    []string{"BOGUS"},
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		policies := ir.Policies{
			Headers: make(map[string]ir.Headers),
		}

		_, err := Routes(serviceIDs, routes, &policies)
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
				Name:    "first",
				Service: "first-service",
				Match: ir.Match{
					PathPrefix: "/a",
				},
			},
			{
				Name:    "second",
				Service: "second-service",
				Match: ir.Match{
					PathPrefix: "/b",
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"first-service":  0,
			"second-service": 1,
		}

		policies := ir.Policies{
			Headers: make(map[string]ir.Headers),
		}

		out, err := Routes(serviceIDs, routes, &policies)
		if err != nil {
			t.Fatal(err)
		}

		if len(out) != 2 {
			t.Fatalf("routes: %#v", out)
		}

		if out[0].Name != "first" || out[0].Service != 0 {
			t.Fatalf("first route: %#v", out[0])
		}

		if out[1].Name != "second" || out[1].Service != 1 {
			t.Fatalf("second route: %#v", out[1])
		}
	})
}
