package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/HAL-X9/aegis/internal/observe/metrics"
)

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

type MetricsMiddleware struct {
	metrics *metrics.Metrics
}

func NewMetricsMiddleware(m *metrics.Metrics) *MetricsMiddleware {
	return &MetricsMiddleware{
		metrics: m,
	}
}

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
			r.Method,
			rec.status,
			elapsed,
		)
	})
}
