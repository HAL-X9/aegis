package public

import (
	"net/http"
	"time"

	"github.com/HAL-X9/aegis/internal/dataplane/middleware"
	"github.com/HAL-X9/aegis/internal/observe/metrics"
)

// Options selects the layers of the public middleware chain.
type Options struct {
	// RequestTimeout bounds the processing of one request. Zero or a negative
	// value disables the layer's deadline.
	RequestTimeout time.Duration

	// RequestID enables the X-Request-Id layer.
	RequestID bool

	// Metrics enables the HTTP metrics layer, recording into this collector.
	// A nil collector leaves the layer out, so a caller that has it turned off
	// need not create (and register) one.
	Metrics *metrics.Metrics
}

// NewHandler returns the public HTTP handler: the executor wrapped in the
// middleware chain selected by opts. From outermost to innermost the layers
// are timeout, metrics, request ID.
func NewHandler(executor RequestExecutor, opts Options) http.Handler {
	handler := NewRouter(NewForwardHandler(executor))

	if opts.RequestID {
		handler = middleware.RequestID(handler)
	}
	if opts.Metrics != nil {
		handler = middleware.NewMetricsMiddleware(opts.Metrics).Metrics(handler)
	}

	return middleware.NewTimeoutMiddleware(opts.RequestTimeout).Timeout(handler)
}
