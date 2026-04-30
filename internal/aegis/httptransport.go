package aegis

import (
	"net"
	"net/http"
	"time"

	"github.com/aegis/internal/config"
)

// newUpstreamTransport builds an http.Transport for outbound requests using
// configured connection limits and network timeouts; the transport is intended
// to be reused across clients.
func newUpstreamTransport(cfg *config.UpstreamTransport) *http.Transport {
	if cfg == nil {
		cfg = &config.UpstreamTransport{}
	}

	return &http.Transport{
		// Connection pooling and per-host limits.
		MaxIdleConns:        cfg.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:     cfg.MaxConnsPerHost,

		// Connection lifecycle and response timeouts.
		IdleConnTimeout:       cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,

		// TCP dialing with timeout and keep-alive.
		DialContext: (&net.Dialer{
			Timeout:   cfg.DialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,

		// Enable HTTP/2 when available.
		ForceAttemptHTTP2: true,
	}
}
