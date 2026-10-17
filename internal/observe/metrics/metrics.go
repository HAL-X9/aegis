package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the gateway's HTTP collectors, all in the "aegis" namespace.
//
// Labels are deliberately bounded: HTTPRequestTotal is labelled by method and
// status, HTTPRequestDuration by method. There is no route or path label yet.
type Metrics struct {
	HTTPRequestTotal    *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
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
		m.HTTPActiveRequests,
	)

	return m
}

// RecordHTTPRequest counts one finished request and observes its duration.
func (m *Metrics) RecordHTTPRequest(method string, statusCode int, duration time.Duration) {
	m.HTTPRequestTotal.WithLabelValues(method, strconv.Itoa(statusCode)).Inc()
	m.HTTPRequestDuration.WithLabelValues(method).Observe(duration.Seconds())
}
