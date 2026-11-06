package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A client's Connection tokens must not delete headers the gateway sets, but
// must still delete the client's own headers (invariant I-009).
func TestExecutorClientConnectionTokensKeepGatewayHeaders(t *testing.T) {
	var got http.Header
	exec := newTestExecutor(t, buildTestEngine(t, pathPolicyConfig()), roundTripFunc(func(r *http.Request) (*http.Response, error) {
		got = r.Header
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))

	req := httptest.NewRequest(http.MethodGet, "http://gateway/public", nil)
	req.Header.Set("Connection", "X-Forwarded-For, "+policyHeader+", X-Client")
	req.Header.Set("X-Client", "dropped")
	exec.ServeHTTP(httptest.NewRecorder(), req)

	if got == nil {
		t.Fatal("request did not reach the upstream")
	}
	if v := got.Get(policyHeader); v != "public" {
		t.Errorf("policy-set %s = %q, want %q", policyHeader, v, "public")
	}
	if got.Get("X-Forwarded-For") == "" {
		t.Error("X-Forwarded-For was removed")
	}
	for _, name := range []string{"Connection", "X-Client"} {
		if _, ok := got[name]; ok {
			t.Errorf("%s was forwarded", name)
		}
	}
}
