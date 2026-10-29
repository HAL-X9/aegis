package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the gateway's HTTP collectors, all in the "aegis" namespace.
//
// Labels are deliberately bounded: HTTPRequestTotal is labelled by method and
// status, HTTPRequestDuration by method, and HTTPErrorsTotal by reason, a
// fixed set of codes. There is no route or path label yet.
type Metrics struct {
	HTTPRequestTotal    *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	// HTTPErrorsTotal counts the error responses the gateway produces itself;
	// an error status relayed from an upstream is not counted.
	HTTPErrorsTotal *prometheus.CounterVec
	// HTTPActiveRequests is registered but never updated, so it always reads 0.
	HTTPActiveRequests prometheus.Gauge
}

// NewMetrics creates the collectors and registers them with reg. Registering
// the same names twice with one registry panics, so call it once per registry.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		HTTPRequestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "aegis",
				Name:      "http_requests_total",
				Help:      "Total number of HTTP request.",
			},
			[]string{"method", "status"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "aegis",
				Name:      "http_request_duration_seconds",
				Help:      "HTTP request duration in seconds.",
				Buckets: []float64{
					0.001,
					0.005,
					0.01,
					0.025,
					0.05,
					0.1,
					0.25,
					0.5,
					1,
					2.5,
					5,
					10,
				},
			},
			[]string{"method"},
		),
		HTTPErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "aegis",
				Name:      "http_errors_total",
				Help:      "Total number of error responses produced by the gateway itself, by reason.",
			},
			[]string{"reason"},
		),
		HTTPActiveRequests: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "aegis",
				Name:      "http_active_requests",
				Help:      "Current number of active HTTP requests.",
			},
		),
	}
	reg.MustRegister(
		m.HTTPRequestTotal,
		m.HTTPRequestDuration,
		m.HTTPErrorsTotal,
		m.HTTPActiveRequests,
	)

	return m
}

// RecordHTTPRequest counts one finished request and observes its duration.
func (m *Metrics) RecordHTTPRequest(method string, statusCode int, duration time.Duration) {
	m.HTTPRequestTotal.WithLabelValues(method, strconv.Itoa(statusCode)).Inc()
	m.HTTPRequestDuration.WithLabelValues(method).Observe(duration.Seconds())
}

// RecordHTTPError counts one error response the gateway produced itself.
func (m *Metrics) RecordHTTPError(reason string) {
	m.HTTPErrorsTotal.WithLabelValues(reason).Inc()
}
