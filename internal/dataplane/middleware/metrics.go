package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/HAL-X9/aegis/internal/httpmethod"
	"github.com/HAL-X9/aegis/internal/observe/metrics"
)

// otherMethod is the method label of every request whose method the gateway
// does not support. The client chooses the method, and each distinct label
// value creates series that live as long as the process, so the raw method
// would let any client exhaust the gateway's memory.
const otherMethod = "_OTHER"

// statusRecorder remembers the first status code written to the response. A
// handler that never calls WriteHeader is reported as 200, matching net/http.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (rec *statusRecorder) WriteHeader(code int) {
	if rec.wroteHeader {
		return
	}
	rec.status = code
	rec.wroteHeader = true
	rec.ResponseWriter.WriteHeader(code)
}

var recorderPool = sync.Pool{
	New: func() any {
		return &statusRecorder{}
	},
}

// MetricsMiddleware records the method, final status and duration of each
// request. It does not label by route: see the routelabel package comment.
type MetricsMiddleware struct {
	metrics *metrics.Metrics
}

// NewMetricsMiddleware returns a middleware that records into m, which must
// not be nil.
func NewMetricsMiddleware(m *metrics.Metrics) *MetricsMiddleware {
	return &MetricsMiddleware{
		metrics: m,
	}
}

// Metrics wraps next, recording each request after next returns.
func (m *MetricsMiddleware) Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recorderPool.Get().(*statusRecorder)

		rec.ResponseWriter = w
		rec.status = http.StatusOK
		rec.wroteHeader = false

		defer recorderPool.Put(rec)

		start := time.Now()

		next.ServeHTTP(rec, r)

		elapsed := time.Since(start)

		m.metrics.RecordHTTPRequest(
			methodLabel(r.Method),
			rec.status,
			elapsed,
		)
	})
}

// methodLabel returns method if the gateway supports it, and otherMethod
// otherwise.
func methodLabel(method string) string {
	if _, ok := httpmethod.Bit(method); ok {
		return method
	}
	return otherMethod
}
