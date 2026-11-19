package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/httpmethod"
	"github.com/HAL-X9/aegis/internal/snapshot"
)

// targetsConfig serves every path through one service with three targets,
// under retry.
func targetsConfig(retry snapshot.CompiledRetry) *snapshot.CompiledConfig {
	return &snapshot.CompiledConfig{
		Services: snapshot.CompiledServices{Items: []snapshot.CompiledService{{
			Name:    "pool",
			Targets: targets("http://a:8080", "http://b:8080", "http://c:8080"),
		}}},
		Routes: []snapshot.CompiledRoute{{
			Name:  "pool",
			Match: snapshot.CompiledMatch{PathPrefix: "/", Methods: httpmethod.All},
			Policies: snapshot.CompiledRoutePolicies{
				RateLimitID: snapshot.NoRateLimit,
				Retry:       retry,
			},
		}},
	}
}

// sent records the URL and Host of every request the transport receives, and
// answers each one with the status that status returns for its host.
type sent struct {
	urls  []string
	hosts []string
}

func (s *sent) transport(status func(host string) int) http.RoundTripper {
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		s.urls = append(s.urls, r.URL.String())
		s.hosts = append(s.hosts, r.Host)
		return &http.Response{StatusCode: status(r.URL.Host), Header: http.Header{}, Body: http.NoBody}, nil
	})
}

func serve(exec *Executor, method, target string) int {
	rec := httptest.NewRecorder()
	exec.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec.Code
}

func TestExecutorTakesTheTargetsInTurn(t *testing.T) {
	var got sent
	exec := newTestExecutor(t, buildTestView(t, targetsConfig(snapshot.CompiledRetry{Attempts: 1})),
		got.transport(func(string) int { return http.StatusOK }))

	for range 4 {
		if code := serve(exec, http.MethodGet, "/items?id=7"); code != http.StatusOK {
			t.Fatalf("status = %d, want 200", code)
		}
	}

	wantURLs := "http://a:8080/items?id=7,http://b:8080/items?id=7,http://c:8080/items?id=7,http://a:8080/items?id=7"
	if urls := strings.Join(got.urls, ","); urls != wantURLs {
		t.Fatalf("upstream URLs = %s, want %s", urls, wantURLs)
	}
	if hosts := strings.Join(got.hosts, ","); hosts != "a:8080,b:8080,c:8080,a:8080" {
		t.Fatalf("Host headers = %s, want each request's target", hosts)
	}
}

// TestExecutorRetriesOnTheNextTarget checks that a retry leaves the failed
// target for the next one in turn, and that the turn of the following
// request is unaffected by the retries of this one.
func TestExecutorRetriesOnTheNextTarget(t *testing.T) {
	var got sent
	exec := newTestExecutor(t, buildTestView(t, targetsConfig(snapshot.CompiledRetry{Attempts: 3})),
		got.transport(func(host string) int {
			if host == "a:8080" {
				return http.StatusServiceUnavailable
			}
			return http.StatusOK
		}))

	if code := serve(exec, http.MethodGet, "/items"); code != http.StatusOK {
		t.Fatalf("first request: status = %d, want 200", code)
	}
	if code := serve(exec, http.MethodGet, "/items"); code != http.StatusOK {
		t.Fatalf("second request: status = %d, want 200", code)
	}

	wantURLs := "http://a:8080/items,http://b:8080/items,http://b:8080/items"
	if urls := strings.Join(got.urls, ","); urls != wantURLs {
		t.Fatalf("upstream URLs = %s, want %s", urls, wantURLs)
	}
	if hosts := strings.Join(got.hosts, ","); hosts != "a:8080,b:8080,b:8080" {
		t.Fatalf("Host headers = %s, want each try's target", hosts)
	}
}

func TestExecutorRetriesWrapAroundTheTargets(t *testing.T) {
	var got sent
	exec := newTestExecutor(t, buildTestView(t, targetsConfig(snapshot.CompiledRetry{Attempts: 5})),
		got.transport(func(string) int { return http.StatusBadGateway }))

	if code := serve(exec, http.MethodGet, "/"); code != http.StatusBadGateway {
		t.Fatalf("status = %d, want the last try's 502", code)
	}

	if hosts := strings.Join(got.hosts, ","); hosts != "a:8080,b:8080,c:8080,a:8080,b:8080" {
		t.Fatalf("tries = %s, want the targets in turn, wrapping around", hosts)
	}
}

func TestExecutorDoesNotMoveANonReplayableRequest(t *testing.T) {
	var got sent
	exec := newTestExecutor(t, buildTestView(t, targetsConfig(snapshot.CompiledRetry{Attempts: 3})),
		got.transport(func(string) int { return http.StatusServiceUnavailable }))

	if code := serve(exec, http.MethodPost, "/"); code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", code)
	}

	if hosts := strings.Join(got.hosts, ","); hosts != "a:8080" {
		t.Fatalf("tries = %s, want a single try", hosts)
	}
}
