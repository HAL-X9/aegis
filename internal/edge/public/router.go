package public

import (
	"net/http"
)

// NewRouter registers public HTTP routes and forwards user traffic to dataplane.
func NewRouter(forward *ForwardHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", forward)
	return mux
}
