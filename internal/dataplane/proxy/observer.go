package proxy

import (
	"net/http"

	"github.com/HAL-X9/aegis/internal/dataplane/fault"
)

// Observer is told about every request the executor answers with its own
// error response rather than with an upstream response. It is called on the
// request's goroutine after the response is written, so it must be safe for
// concurrent use and should return quickly.
type Observer interface {
	Failed(r *http.Request, failure Failure)
}

// Failure describes one error response produced by the executor.
type Failure struct {
	Reason fault.Reason

	// Route is the name of the matched route, or "" when the request failed
	// before a route matched.
	Route string

	// Cause is the upstream error behind UpstreamTimeout and UpstreamFailed;
	// it is nil for every other reason.
	Cause error
}

type discardObserver struct{}

func (discardObserver) Failed(*http.Request, Failure) {}

// fail answers r with the problem document of failure.Reason and reports the
// failure. It is the only way ServeHTTP produces an error response.
func (executor *Executor) fail(w http.ResponseWriter, r *http.Request, failure Failure) {
	fault.Write(w, failure.Reason)
	executor.observer.Failed(r, failure)
}
