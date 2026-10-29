package app

import (
	"log"
	"net/http"

	"github.com/HAL-X9/aegis/internal/dataplane/proxy"
	"github.com/HAL-X9/aegis/internal/observe/metrics"
)

// failureReporter is the proxy.Observer of the gateway. Every error response
// is counted by reason when metrics are enabled. Only the gateway's own
// faults, the 5xx reasons, are also logged with their cause: client errors
// are left out of the log, so that a client cannot flood it.
type failureReporter struct {
	// metrics is nil when the metrics middleware is disabled.
	metrics *metrics.Metrics
}

func (fr failureReporter) Failed(r *http.Request, failure proxy.Failure) {
	if fr.metrics != nil {
		fr.metrics.RecordHTTPError(failure.Reason.String())
	}

	if failure.Reason.Status() < http.StatusInternalServerError {
		return
	}
	log.Printf("aegis: gateway error: reason=%s route=%q method=%s path=%q request_id=%q cause=%v",
		failure.Reason, failure.Route, r.Method, r.URL.Path, r.Header.Get("X-Request-Id"), failure.Cause)
}
