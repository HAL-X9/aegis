package proxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/controlplane/snapshot"
	"github.com/HAL-X9/aegis/internal/dataplane/policy"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func buildTestEngine(t *testing.T, cfg *snapshot.CompiledConfig) *router.Engine {
	t.Helper()

	engine, err := router.BuildEngine(cfg)
	if err != nil {
		t.Fatalf("BuildEngine failed: %v", err)
	}

	return engine
}

// noRateLimits returns a RateLimiterSet with no configured policies, so
// Allow always permits the request. Used by tests and benchmarks in this
// package that don't exercise rate-limiting behavior directly.
func noRateLimits() *policy.RateLimiterSet {
	return policy.NewRateLimiterSet(nil)
}

func testConfig(routes ...snapshot.CompiledRoute) *snapshot.CompiledConfig {
	return &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{
			Items: []snapshot.CompiledService{
				{
					Name:     "upstream",
					Upstream: "http://upstream:8080",
				},
				{
					Name:     "ignored",
					Upstream: "http://ignored:8080",
				},
				{
					Name:     "fallback",
					Upstream: "http://upstream:9090",
				},
			},
		},
		Routes: routes,
	}
}

func TestExecutor(t *testing.T) {
	t.Run("returns 503 when engine is nil", func(t *testing.T) {
		exec := NewExecutor(nil, noRateLimits(), roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("should not be called")
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("returns 500 when transport is nil", func(t *testing.T) {
		engine := buildTestEngine(t, testConfig(
			snapshot.CompiledRoute{
				Name:    "api",
				Service: 0,
				Match: snapshot.CompiledMatch{
					PathPrefix: "/api",
					Methods:    methodmask.MethodAll,
				},
			},
		))

		exec := NewExecutor(engine, noRateLimits(), nil)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})

	t.Run("returns 404 when path has no matching route", func(t *testing.T) {
		engine := buildTestEngine(t, testConfig(
			snapshot.CompiledRoute{
				Name:    "api",
				Service: 0,
				Match: snapshot.CompiledMatch{
					PathPrefix: "/api",
					Methods:    methodmask.MethodAll,
				},
			},
		))

		exec := NewExecutor(engine, noRateLimits(), roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("should not be called")
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/other", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("returns 405 for unsupported incoming HTTP method", func(t *testing.T) {
		engine := buildTestEngine(t, testConfig(
			snapshot.CompiledRoute{
				Name:    "api",
				Service: 0,
				Match: snapshot.CompiledMatch{
					PathPrefix: "/api",
					Methods:    methodmask.MethodAll,
				},
			},
		))

		exec := NewExecutor(engine, noRateLimits(), roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("should not be called")
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("TRACE", "/api", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("returns 405 when route matches path but not method", func(t *testing.T) {
		getMask, ok := methodmask.MethodBit(http.MethodGet)
		if !ok {
			t.Fatal("failed to get method bit for GET")
		}

		engine := buildTestEngine(t, testConfig(
			snapshot.CompiledRoute{
				Name:    "api",
				Service: 0,
				Match: snapshot.CompiledMatch{
					PathPrefix: "/api",
					Methods:    getMask,
				},
			},
		))

		exec := NewExecutor(engine, noRateLimits(), roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("should not be called")
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("returns 404 when method matches but headers do not", func(t *testing.T) {
		engine := buildTestEngine(t, testConfig(
			snapshot.CompiledRoute{
				Name:    "api",
				Service: 0,
				Match: snapshot.CompiledMatch{
					PathPrefix: "/api",
					Methods:    methodmask.MethodAll,
					Headers: []snapshot.HeaderPredicate{
						{
							Name:          "X-Tenant",
							AllowedValues: []string{"a"},
						},
					},
				},
			},
		))

		exec := NewExecutor(engine, noRateLimits(), roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("should not be called")
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Set("X-Tenant", "b")

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("returns 502 when upstream call fails", func(t *testing.T) {
		engine := buildTestEngine(t, testConfig(
			snapshot.CompiledRoute{
				Name:    "api",
				Service: 0,
				Match: snapshot.CompiledMatch{
					PathPrefix: "/api",
					Methods:    methodmask.MethodAll,
				},
			},
		))

		exec := NewExecutor(engine, noRateLimits(), roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("dial failure")
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
		}
	})

	t.Run("preserves query parameters when building upstream target", func(t *testing.T) {
		engine := buildTestEngine(t, testConfig(
			snapshot.CompiledRoute{
				Name:    "api",
				Service: 0,
				Match: snapshot.CompiledMatch{
					PathPrefix: "/api/profile",
					Methods:    methodmask.MethodAll,
				},
			},
		))

		var gotURL string

		exec := NewExecutor(engine, noRateLimits(), roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotURL = r.URL.String()

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     make(http.Header),
			}, nil
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodGet,
			"/api/profile?id=123&sort=name",
			nil,
		)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		if gotURL != "http://upstream:8080/api/profile?id=123&sort=name" {
			t.Fatalf(
				"upstream URL = %q, want %q",
				gotURL,
				"http://upstream:8080/api/profile?id=123&sort=name",
			)
		}
	})

	t.Run("proxies response for first matching route", func(t *testing.T) {
		engine := buildTestEngine(t, testConfig(
			snapshot.CompiledRoute{
				Name:    "restricted",
				Service: 1,
				Match: snapshot.CompiledMatch{
					PathPrefix: "/api",
					Methods:    methodmask.MethodAll,
					Headers: []snapshot.HeaderPredicate{
						{
							Name:          "X-Tenant",
							AllowedValues: []string{"a"},
						},
					},
				},
			},
			snapshot.CompiledRoute{
				Name:    "fallback",
				Service: 2,
				Match: snapshot.CompiledMatch{
					PathPrefix: "/api",
					Methods:    methodmask.MethodAll,
				},
			},
		))

		var gotMethod string
		var gotURL string
		var gotBody string

		exec := NewExecutor(engine, noRateLimits(), roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotMethod = r.Method
			gotURL = r.URL.String()

			b, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			gotBody = string(b)

			return &http.Response{
				StatusCode: http.StatusCreated,
				Body:       io.NopCloser(strings.NewReader("upstream ok")),
				Header:     make(http.Header),
			}, nil
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(
			http.MethodGet,
			"/api",
			strings.NewReader("payload"),
		)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
		}

		if rec.Body.String() != "upstream ok" {
			t.Fatalf("body = %q, want %q", rec.Body.String(), "upstream ok")
		}

		if gotMethod != http.MethodGet {
			t.Fatalf("method = %q, want %q", gotMethod, http.MethodGet)
		}

		if gotURL != "http://upstream:9090/api" {
			t.Fatalf("target URL = %q", gotURL)
		}

		if gotBody != "payload" {
			t.Fatalf("body = %q, want %q", gotBody, "payload")
		}
	})
}
