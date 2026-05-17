package proxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aegis/internal/controlplane/representation/source"
	"github.com/aegis/internal/dataplane/router"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func buildTestEngine(t *testing.T, cfg *source.GatewayConfig) *router.Engine {
	t.Helper()
	engine, err := router.BuildEngine(cfg)
	if err != nil {
		t.Fatalf("BuildEngine failed: %v", err)
	}
	return engine
}

func TestExecutor(t *testing.T) {
	t.Run("returns 503 when engine is nil", func(t *testing.T) {
		exec := NewExecutor(nil, roundTripFunc(func(r *http.Request) (*http.Response, error) {
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
		engine := buildTestEngine(t, &source.GatewayConfig{
			Routes: []source.Route{
				{
					Name:     "api",
					Match:    source.Match{PathPrefix: "/api"},
					Upstream: source.Upstream{Scheme: "http", Host: "upstream", Port: 8080},
				},
			},
		})
		exec := NewExecutor(engine, nil)
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})

	t.Run("returns 404 when path has no matching route", func(t *testing.T) {
		engine := buildTestEngine(t, &source.GatewayConfig{
			Routes: []source.Route{
				{
					Name:     "api",
					Match:    source.Match{PathPrefix: "/api"},
					Upstream: source.Upstream{Scheme: "http", Host: "upstream", Port: 8080},
				},
			},
		})
		exec := NewExecutor(engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
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
		engine := buildTestEngine(t, &source.GatewayConfig{
			Routes: []source.Route{
				{
					Name:     "api",
					Match:    source.Match{PathPrefix: "/api"},
					Upstream: source.Upstream{Scheme: "http", Host: "upstream", Port: 8080},
				},
			},
		})
		exec := NewExecutor(engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
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
		engine := buildTestEngine(t, &source.GatewayConfig{
			Routes: []source.Route{
				{
					Name:     "api",
					Match:    source.Match{PathPrefix: "/api", Methods: []string{http.MethodGet}},
					Upstream: source.Upstream{Scheme: "http", Host: "upstream", Port: 8080},
				},
			},
		})
		exec := NewExecutor(engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
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
		engine := buildTestEngine(t, &source.GatewayConfig{
			Routes: []source.Route{
				{
					Name: "api",
					Match: source.Match{
						PathPrefix: "/api",
						Methods:    []string{http.MethodGet},
						Headers: map[string][]string{
							"X-Tenant": {"a"},
						},
					},
					Upstream: source.Upstream{Scheme: "http", Host: "upstream", Port: 8080},
				},
			},
		})
		exec := NewExecutor(engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
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
		engine := buildTestEngine(t, &source.GatewayConfig{
			Routes: []source.Route{
				{
					Name:     "api",
					Match:    source.Match{PathPrefix: "/api", Methods: []string{http.MethodGet}},
					Upstream: source.Upstream{Scheme: "http", Host: "upstream", Port: 8080},
				},
			},
		})
		exec := NewExecutor(engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("dial failure")
		}))
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api", nil)

		exec.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
		}
	})

	t.Run("proxies response for first matching route", func(t *testing.T) {
		engine := buildTestEngine(t, &source.GatewayConfig{
			Routes: []source.Route{
				{
					Name: "restricted",
					Match: source.Match{
						PathPrefix: "/api",
						Methods:    []string{http.MethodGet},
						Headers: map[string][]string{
							"X-Tenant": {"a"},
						},
					},
					Upstream: source.Upstream{Scheme: "http", Host: "ignored", Port: 8080},
				},
				{
					Name:     "fallback",
					Match:    source.Match{PathPrefix: "/api", Methods: []string{http.MethodGet}},
					Upstream: source.Upstream{Scheme: "http", Host: "upstream", Port: 9090},
				},
			},
		})

		var gotMethod string
		var gotURL string
		var gotBody string

		exec := NewExecutor(engine, roundTripFunc(func(r *http.Request) (*http.Response, error) {
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
		req := httptest.NewRequest(http.MethodGet, "/api", strings.NewReader("payload"))

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
