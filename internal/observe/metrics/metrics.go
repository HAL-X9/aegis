package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	HTTPRequestTotal    *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPActiveRequests  prometheus.Gauge
}

func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		HTTPRequestTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "aegis",
				Name:      "http_requests_total",
				Help:      "Total number of HTTP request.",
			},
			[]string{"method", "route", "status"},
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
			[]string{"method", "route"},
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

func (m *Metrics) RecordHTTPRequest(method, route string, statusCode int, duration time.Duration) {
	m.HTTPRequestTotal.WithLabelValues(method, route, strconv.Itoa(statusCode)).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, route).Observe(duration.Seconds())
}
