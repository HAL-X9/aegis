package public

import "net/http"

// NewRouter returns the public HTTP handler.
//
// Public routing is implemented by the dataplane. The public edge only
// provides the HTTP entrypoint and forwards requests to the dataplane.
func NewRouter(forward *ForwardHandler) http.Handler {
	return forward
}
