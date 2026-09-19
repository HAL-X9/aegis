package proxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/dataplane/policy"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
	"github.com/HAL-X9/aegis/internal/snapshot"
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

func newTestExecutor(
	t *testing.T,
	engine *router.Engine,
	transport http.RoundTripper,
) *Executor {
	t.Helper()

	exec, err := NewExecutor(transport)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	exec.Publish(&View{
		Engine:   engine,
		Limiters: noRateLimits(),
	})

	return exec
}

func TestNewExecutor(t *testing.T) {
	t.Run("rejects nil transport", func(t *testing.T) {
		exec, err := NewExecutor(nil)

		if err == nil {
			t.Fatal("expected error for nil transport")
		}

		if exec != nil {
			t.Fatal("expected nil executor on constructor error")
		}
	})

	t.Run("accepts non-nil transport", func(t *testing.T) {
		transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("not called")
		})

		exec, err := NewExecutor(transport)
		if err != nil {
			t.Fatalf("NewExecutor failed: %v", err)
		}

		if exec == nil {
			t.Fatal("expected non-nil executor")
		}
	})
}

func TestExecutor(t *testing.T) {
	t.Run("returns 503 before a view is published", func(t *testing.T) {
		exec, err := NewExecutor(roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("should not be called")
		}))
		if err != nil {
			t.Fatalf("NewExecutor failed: %v", err)
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf(
				"status = %d, want %d",
				rec.Code,
				http.StatusServiceUnavailable,
			)
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

		exec := newTestExecutor(t, engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("should not be called")
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/other", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf(
				"status = %d, want %d",
				rec.Code,
				http.StatusNotFound,
			)
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

		exec := newTestExecutor(t, engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("should not be called")
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("TRACE", "/api", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf(
				"status = %d, want %d",
				rec.Code,
				http.StatusMethodNotAllowed,
			)
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

		exec := newTestExecutor(t, engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("should not be called")
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf(
				"status = %d, want %d",
				rec.Code,
				http.StatusMethodNotAllowed,
			)
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

		exec := newTestExecutor(t, engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("should not be called")
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Set("X-Tenant", "b")

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf(
				"status = %d, want %d",
				rec.Code,
				http.StatusNotFound,
			)
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

		exec := newTestExecutor(t, engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("dial failure")
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf(
				"status = %d, want %d",
				rec.Code,
				http.StatusBadGateway,
			)
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

		exec := newTestExecutor(t, engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
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
			t.Fatalf(
				"status = %d, want %d",
				rec.Code,
				http.StatusOK,
			)
		}

		wantURL := "http://upstream:8080/api/profile?id=123&sort=name"
		if gotURL != wantURL {
			t.Fatalf(
				"upstream URL = %q, want %q",
				gotURL,
				wantURL,
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

		exec := newTestExecutor(t, engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotMethod = r.Method
			gotURL = r.URL.String()

			body, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			gotBody = string(body)

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
			t.Fatalf(
				"status = %d, want %d",
				rec.Code,
				http.StatusCreated,
			)
		}

		if rec.Body.String() != "upstream ok" {
			t.Fatalf(
				"body = %q, want %q",
				rec.Body.String(),
				"upstream ok",
			)
		}

		if gotMethod != http.MethodGet {
			t.Fatalf(
				"method = %q, want %q",
				gotMethod,
				http.MethodGet,
			)
		}

		if gotURL != "http://upstream:9090/api" {
			t.Fatalf("target URL = %q", gotURL)
		}

		if gotBody != "payload" {
			t.Fatalf(
				"body = %q, want %q",
				gotBody,
				"payload",
			)
		}
	})

	t.Run("strips hop-by-hop headers from upstream response", func(t *testing.T) {
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

		exec := newTestExecutor(t, engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
			h := make(http.Header)
			h.Set("Connection", "close")
			h.Set("Transfer-Encoding", "chunked")
			h.Set("Content-Type", "text/plain")

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
				Header:     h,
			}, nil
		}))

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api", nil)

		exec.ServeHTTP(rec, req)

		if got := rec.Header().Get("Connection"); got != "" {
			t.Errorf("Connection leaked to client: %q", got)
		}

		if got := rec.Header().Get("Transfer-Encoding"); got != "" {
			t.Errorf("Transfer-Encoding leaked to client: %q", got)
		}

		if got := rec.Header().Get("Content-Type"); got != "text/plain" {
			t.Errorf(
				"Content-Type = %q, want preserved",
				got,
			)
		}
	})
}
