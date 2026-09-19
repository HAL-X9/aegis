package compile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/controlplane/ir"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

func newTestHeaderRegistry() *HeaderRegistryBuilder {
	return NewHeaderRegistryBuilder()
}

func TestRoutes(t *testing.T) {
	t.Run("empty routes", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

		out, err := Routes(headerIDs, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}

		if len(out) != 0 {
			t.Fatalf("got %#v", out)
		}
	})

	t.Run("single route", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

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

		out, err := Routes(headerIDs, serviceIDs, routes, nil)
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
					RateLimitID: -1,
				},
			},
		}

		if !reflect.DeepEqual(out, want) {
			t.Errorf("diff (-got +want)\ngot:  %+v\nwant: %+v", out, want)
		}
	})

	t.Run("unknown service", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

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

		_, err := Routes(headerIDs, serviceIDs, routes, nil)
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
		headerIDs := newTestHeaderRegistry()

		routes := []ir.Route{
			{
				Name:    "any",
				Service: "api-service",
				Match: ir.Match{
					PathPrefix: "/",
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		policies := ir.Policies{
			Headers: make(map[string]ir.Headers),
		}

		out, err := Routes(headerIDs, serviceIDs, routes, &policies)
		if err != nil {
			t.Fatal(err)
		}

		if got := out[0].Match.Methods; got != methodmask.MethodAll {
			t.Fatalf("Methods = %#v, want MethodAll (%#v)", got, methodmask.MethodAll)
		}

		if got := out[0].Policies.RateLimitID; got != -1 {
			t.Fatalf("RateLimitID = %d, want -1", got)
		}
	})

	t.Run("invalid HTTP method", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

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

		_, err := Routes(headerIDs, serviceIDs, routes, &policies)
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
		headerIDs := newTestHeaderRegistry()

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

		out, err := Routes(headerIDs, serviceIDs, routes, &policies)
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

func TestRoutes_headersPredicate(t *testing.T) {
	t.Run("headers are sorted deterministically", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

		routes := []ir.Route{
			{
				Name:    "api",
				Service: "api-service",
				Match: ir.Match{
					PathPrefix: "/",
					Headers: map[string][]string{
						"X-Request-ID": {"request-1"},
						"Authorization": {
							"Bearer token",
							"Basic credentials",
						},
						"Host": nil,
					},
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		out, err := Routes(headerIDs, serviceIDs, routes, nil)
		if err != nil {
			t.Fatal(err)
		}

		got := out[0].Match.Headers

		if len(got) != 3 {
			t.Fatalf("expected 3 predicates, got %d: %+v", len(got), got)
		}

		if got[0].Name != "Authorization" {
			t.Errorf("first predicate = %q, want Authorization", got[0].Name)
		}
		if got[1].Name != "Host" {
			t.Errorf("second predicate = %q, want Host", got[1].Name)
		}
		if got[2].Name != "X-Request-ID" {
			t.Errorf("third predicate = %q, want X-Request-ID", got[2].Name)
		}

		if !reflect.DeepEqual(
			got[0].AllowedValues,
			[]string{"Bearer token", "Basic credentials"},
		) {
			t.Errorf(
				"Authorization values = %v, want %v",
				got[0].AllowedValues,
				[]string{"Bearer token", "Basic credentials"},
			)
		}

		if got[1].AllowedValues != nil {
			t.Errorf("Host values = %v, want nil", got[1].AllowedValues)
		}
	})

	t.Run("duplicate allowed values are removed", func(t *testing.T) {
		got, err := headersPredicate(map[string][]string{
			"Authorization": {
				"Bearer token",
				"Bearer token",
				"Basic credentials",
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		want := []snapshot.HeaderPredicate{
			{
				Name: "Authorization",
				AllowedValues: []string{
					"Bearer token",
					"Basic credentials",
				},
			},
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("empty header name returns error", func(t *testing.T) {
		_, err := headersPredicate(map[string][]string{
			"": {"value"},
		})
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "header name cannot be empty") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty allowed value returns error", func(t *testing.T) {
		_, err := headersPredicate(map[string][]string{
			"Authorization": {"Bearer token", ""},
		})
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), `header "Authorization" contains empty allowed value`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRoutes_policyReferences(t *testing.T) {
	t.Run("unknown policy returns error", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

		routes := []ir.Route{
			{
				Name:    "api",
				Service: "api-service",
				Policies: []ir.PolicyRef{
					{Name: "missing"},
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		policies := &ir.Policies{}

		_, err := Routes(headerIDs, serviceIDs, routes, policies)
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), `route "api"`) {
			t.Fatalf("error should mention route: %v", err)
		}

		if !strings.Contains(err.Error(), `references unknown policy "missing"`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("references require policies", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

		routes := []ir.Route{
			{
				Name:    "api",
				Service: "api-service",
				Policies: []ir.PolicyRef{
					{Name: "auth"},
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		_, err := Routes(headerIDs, serviceIDs, routes, nil)
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "policy references require normalized policies") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRoutes_headerPolicies(t *testing.T) {
	t.Run("header policy is merged into route", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

		routes := []ir.Route{
			{
				Name:    "api",
				Service: "api-service",
				Policies: []ir.PolicyRef{
					{Name: "security"},
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		policies := &ir.Policies{
			Headers: map[string]ir.Headers{
				"security": {
					Request: ir.HeadersOps{
						Set: map[string]string{
							"Authorization": "internal",
						},
					},
					Response: ir.HeadersOps{
						Add: map[string]string{
							"X-Frame-Options": "DENY",
						},
					},
				},
			},
		}

		out, err := Routes(headerIDs, serviceIDs, routes, policies)
		if err != nil {
			t.Fatal(err)
		}

		headers := out[0].Policies.Headers

		if len(headers.Request.Ops) != 1 {
			t.Fatalf("expected 1 request op, got %d", len(headers.Request.Ops))
		}

		if headers.Request.Ops[0].Op != snapshot.HeaderOpSet {
			t.Errorf(
				"request op = %v, want HeaderOpSet",
				headers.Request.Ops[0].Op,
			)
		}

		if headers.Request.Ops[0].HeaderID != snapshot.HeaderAuthorization {
			t.Errorf(
				"request HeaderID = %v, want %v",
				headers.Request.Ops[0].HeaderID,
				snapshot.HeaderAuthorization,
			)
		}

		if headers.Request.Ops[0].Value != "internal" {
			t.Errorf(
				"request Value = %q, want %q",
				headers.Request.Ops[0].Value,
				"internal",
			)
		}

		if len(headers.Response.Ops) != 1 {
			t.Fatalf("expected 1 response op, got %d", len(headers.Response.Ops))
		}

		if headers.Response.Ops[0].Op != snapshot.HeaderOpAddIfAbsent {
			t.Errorf(
				"response op = %v, want HeaderOpAddIfAbsent",
				headers.Response.Ops[0].Op,
			)
		}

		if headers.Response.Ops[0].HeaderID != snapshot.HeaderXFrameOptions {
			t.Errorf(
				"response HeaderID = %v, want %v",
				headers.Response.Ops[0].HeaderID,
				snapshot.HeaderXFrameOptions,
			)
		}
	})

	t.Run("conflicting header policies return error", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

		routes := []ir.Route{
			{
				Name:    "api",
				Service: "api-service",
				Policies: []ir.PolicyRef{
					{Name: "first"},
					{Name: "second"},
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		policies := &ir.Policies{
			Headers: map[string]ir.Headers{
				"first": {
					Request: ir.HeadersOps{
						Set: map[string]string{
							"Authorization": "first",
						},
					},
				},
				"second": {
					Request: ir.HeadersOps{
						Set: map[string]string{
							"Authorization": "second",
						},
					},
				},
			},
		}

		_, err := Routes(headerIDs, serviceIDs, routes, policies)
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "conflicting operations") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unknown referenced header policy is rejected before compilation", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

		routes := []ir.Route{
			{
				Name:    "api",
				Service: "api-service",
				Policies: []ir.PolicyRef{
					{Name: "missing"},
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		policies := &ir.Policies{
			Headers: map[string]ir.Headers{},
		}

		_, err := Routes(headerIDs, serviceIDs, routes, policies)
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), `references unknown policy "missing"`) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRoutes_rateLimitPolicy(t *testing.T) {
	t.Run("route receives compiled rate limit ID", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

		routes := []ir.Route{
			{
				Name:    "api",
				Service: "api-service",
				Policies: []ir.PolicyRef{
					{Name: "slow"},
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		policies := &ir.Policies{
			RateLimits: map[string]ir.RateLimit{
				"slow": {
					Rate:  10,
					Burst: 5,
				},
			},
		}

		out, err := Routes(headerIDs, serviceIDs, routes, policies)
		if err != nil {
			t.Fatal(err)
		}

		if got := out[0].Policies.RateLimitID; got != 0 {
			t.Fatalf("RateLimitID = %d, want 0", got)
		}
	})

	t.Run("multiple rate limits are rejected", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

		routes := []ir.Route{
			{
				Name:    "api",
				Service: "api-service",
				Policies: []ir.PolicyRef{
					{Name: "slow"},
					{Name: "fast"},
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		policies := &ir.Policies{
			RateLimits: map[string]ir.RateLimit{
				"fast": {
					Rate:  100,
					Burst: 10,
				},
				"slow": {
					Rate:  10,
					Burst: 5,
				},
			},
		}

		_, err := Routes(headerIDs, serviceIDs, routes, policies)
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), "multiple rate-limit policies") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rate limit IDs follow sorted policy names", func(t *testing.T) {
		headerIDs := newTestHeaderRegistry()

		routes := []ir.Route{
			{
				Name:    "api",
				Service: "api-service",
				Policies: []ir.PolicyRef{
					{Name: "slow"},
				},
			},
		}

		serviceIDs := map[string]snapshot.ServiceID{
			"api-service": 0,
		}

		policies := &ir.Policies{
			RateLimits: map[string]ir.RateLimit{
				"z-limit": {
					Rate:  30,
					Burst: 3,
				},
				"slow": {
					Rate:  10,
					Burst: 1,
				},
			},
		}

		out, err := Routes(headerIDs, serviceIDs, routes, policies)
		if err != nil {
			t.Fatal(err)
		}

		// "slow" sorts before "z-limit", so its ID is zero.
		if got := out[0].Policies.RateLimitID; got != 0 {
			t.Fatalf("RateLimitID = %d, want 0", got)
		}
	})
}

func TestMergeHeadersOps(t *testing.T) {
	t.Run("remove then set then add are preserved", func(t *testing.T) {
		dst := ir.HeadersOps{}
		src := ir.HeadersOps{
			Remove: []string{"Server"},
			Set: map[string]string{
				"Content-Type": "application/json",
			},
			Add: map[string]string{
				"X-Frame-Options": "DENY",
			},
		}

		if err := mergeHeadersOps(&dst, &src); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(dst.Remove, []string{"Server"}) {
			t.Errorf("Remove = %v, want [Server]", dst.Remove)
		}

		if !reflect.DeepEqual(
			dst.Set,
			map[string]string{"Content-Type": "application/json"},
		) {
			t.Errorf("Set = %v", dst.Set)
		}

		if !reflect.DeepEqual(
			dst.Add,
			map[string]string{"X-Frame-Options": "DENY"},
		) {
			t.Errorf("Add = %v", dst.Add)
		}
	})

	t.Run("same header across operations conflicts", func(t *testing.T) {
		tests := []struct {
			name string
			dst  ir.HeadersOps
			src  ir.HeadersOps
		}{
			{
				name: "remove vs set",
				dst: ir.HeadersOps{
					Remove: []string{"Server"},
				},
				src: ir.HeadersOps{
					Set: map[string]string{"Server": "nginx"},
				},
			},
			{
				name: "set vs add",
				dst: ir.HeadersOps{
					Set: map[string]string{"Server": "nginx"},
				},
				src: ir.HeadersOps{
					Add: map[string]string{"Server": "nginx"},
				},
			},
			{
				name: "add vs remove",
				dst: ir.HeadersOps{
					Add: map[string]string{"Server": "nginx"},
				},
				src: ir.HeadersOps{
					Remove: []string{"Server"},
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := mergeHeadersOps(&tt.dst, &tt.src); err == nil {
					t.Fatal("expected conflict error")
				} else if !strings.Contains(err.Error(), "conflicting operations") {
					t.Fatalf("unexpected error: %v", err)
				}
			})
		}
	})
}
