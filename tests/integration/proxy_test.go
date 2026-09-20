package integration

import (
	"net/http"
	"testing"

	"github.com/HAL-X9/aegis/internal/contracts/methodmask"
	"github.com/HAL-X9/aegis/internal/dataplane/router"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

// buildTestEngine compiles a single route requiring GET and a present
// Authorization header, mirroring the shape the real control-plane
// compiler produces for one route entry in gateway.yaml.
func buildTestEngine(t *testing.T) *router.Engine {
	t.Helper()

	getBit, ok := methodmask.MethodBit(http.MethodGet)
	if !ok {
		t.Fatalf("methodmask.MethodBit(%q): not recognized", http.MethodGet)
	}

	cfg := &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{
			Items: []snapshot.CompiledService{
				{Name: "user-profile", Upstream: "http://127.0.0.1:0"},
			},
		},
		Routes: []snapshot.CompiledRoute{
			{
				Name:    "user-profile",
				Service: 0,
				Match: snapshot.CompiledMatch{
					PathPrefix: "/api/v1/profile",
					Methods:    getBit,
					Headers: []snapshot.HeaderPredicate{
						{Name: "Authorization"},
					},
				},
			},
		},
	}

	engine, err := router.BuildEngine(cfg)
	if err != nil {
		t.Fatalf("BuildEngine: %v", err)
	}
	return engine
}

// resolve mirrors proxy.Executor.ServeHTTP's matching loop closely enough
// to observe the same outcome (path miss / method miss / header miss /
// full match) without depending on the executor's HTTP wiring.
func resolve(engine *router.Engine, method, path string, headers http.Header) (pathMatched, methodMatched bool, matched *snapshot.CompiledRoute) {
	methodBit, ok := methodmask.MethodBit(method)
	if !ok {
		return false, false, nil
	}

	engine.Lookup(path, func(candidateIDs []snapshot.RouteID) bool {
		pathMatched = true
		for _, id := range candidateIDs {
			route := engine.Route(id)
			if route.Match.Methods&methodBit == 0 {
				continue
			}
			methodMatched = true
			if router.HeadersMatch(route.Match.Headers, headers) {
				matched = route
				return true
			}
		}
		return false
	})
	return pathMatched, methodMatched, matched
}

func TestRouting_MatchesPathMethodAndHeaders(t *testing.T) {
	engine := buildTestEngine(t)
	headers := http.Header{"Authorization": []string{"Bearer token"}}

	pathMatched, methodMatched, matched := resolve(engine, http.MethodGet, "/api/v1/profile/123", headers)

	if !pathMatched || !methodMatched || matched == nil {
		t.Fatalf("pathMatched=%v methodMatched=%v matched=%v, want full match", pathMatched, methodMatched, matched)
	}
	if matched.Name != "user-profile" {
		t.Errorf("matched route = %q, want %q", matched.Name, "user-profile")
	}
}

func TestRouting_UnknownPathIsNotFound(t *testing.T) {
	engine := buildTestEngine(t)

	pathMatched, _, matched := resolve(engine, http.MethodGet, "/does/not/exist", http.Header{})

	if pathMatched || matched != nil {
		t.Fatalf("pathMatched=%v matched=%v, want no structural match at all", pathMatched, matched)
	}
}

func TestRouting_WrongMethodIsMethodNotAllowed(t *testing.T) {
	engine := buildTestEngine(t)
	headers := http.Header{"Authorization": []string{"Bearer token"}}

	pathMatched, methodMatched, matched := resolve(engine, http.MethodPost, "/api/v1/profile", headers)

	if !pathMatched || methodMatched || matched != nil {
		t.Fatalf("pathMatched=%v methodMatched=%v matched=%v, want path match with no method support", pathMatched, methodMatched, matched)
	}
}

func TestRouting_MissingRequiredHeaderIsNotFound(t *testing.T) {
	engine := buildTestEngine(t)

	pathMatched, methodMatched, matched := resolve(engine, http.MethodGet, "/api/v1/profile", http.Header{})

	if !pathMatched || !methodMatched || matched != nil {
		t.Fatalf("pathMatched=%v methodMatched=%v matched=%v, want method ok but header predicate to fail (see docs/routing-and-errors.md)", pathMatched, methodMatched, matched)
	}
}
