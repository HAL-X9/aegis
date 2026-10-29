// Package fault defines the error responses the gateway produces itself,
// as opposed to the responses it relays from an upstream.
//
// Each Reason fixes, in one table, the HTTP status, the stable code that
// names the reason in the problem type and in metrics, and the body: a
// problem document (RFC 9457). The documents are encoded once, when the
// package is initialized, so Write neither encodes nor allocates.
package fault

import (
	"encoding/json"
	"net/http"
)

// Reason is why the gateway answered a request itself. Only the constants
// below are valid reasons.
type Reason uint8

// The reasons, in the order the executor decides them
// (docs/routing-and-errors.md §2).
const (
	NotReady Reason = iota + 1
	InvalidPath
	UnsupportedMethod
	NoRoute
	MethodNotAllowed
	RateLimited
	UpstreamTimeout
	UpstreamFailed

	numReasons
)

type definition struct {
	status int
	code   string
	body   []byte
}

var definitions = [numReasons]definition{
	NotReady:          define(http.StatusServiceUnavailable, "not-ready", "Gateway is not ready"),
	InvalidPath:       define(http.StatusBadRequest, "invalid-path", "Request path has no canonical form"),
	UnsupportedMethod: define(http.StatusMethodNotAllowed, "unsupported-method", "Request method is not supported"),
	NoRoute:           define(http.StatusNotFound, "no-route", "No route matches the request"),
	MethodNotAllowed:  define(http.StatusMethodNotAllowed, "method-not-allowed", "Route does not allow the request method"),
	RateLimited:       define(http.StatusTooManyRequests, "rate-limited", "Rate limit exceeded"),
	UpstreamTimeout:   define(http.StatusGatewayTimeout, "upstream-timeout", "Upstream did not respond in time"),
	UpstreamFailed:    define(http.StatusBadGateway, "upstream-failed", "Upstream request failed"),
}

// typePrefix turns a code into the problem type, an absolute URI that
// identifies the reason without having to be dereferenceable.
const typePrefix = "urn:aegis:problem:"

func define(status int, code, title string) definition {
	body, err := json.Marshal(struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status int    `json:"status"`
	}{typePrefix + code, title, status})
	if err != nil {
		panic(err) // unreachable: strings and an int always encode
	}
	return definition{status: status, code: code, body: body}
}

// Status returns the HTTP status code the gateway answers with.
func (r Reason) Status() int {
	return definitions[r].status
}

// String returns the stable code of the reason, such as "no-route". The code
// names the reason in the problem type and in the reason label of metrics,
// so changing it is a contract change.
func (r Reason) String() string {
	return definitions[r].code
}

// Header values shared by every problem response. Sharing is safe: header
// value slices are replaced or appended to, never written in place.
var (
	contentType = []string{"application/problem+json"}
	noSniff     = []string{"nosniff"}
)

// Write sends the problem document of reason as the whole response. Nothing
// may have been written to w before.
func Write(w http.ResponseWriter, reason Reason) {
	d := &definitions[reason]

	h := w.Header()
	h["Content-Type"] = contentType
	h["X-Content-Type-Options"] = noSniff

	w.WriteHeader(d.status)
	_, _ = w.Write(d.body)
}
