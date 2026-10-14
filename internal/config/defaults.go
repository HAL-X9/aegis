package config

import "time"

// Header size limits applied when a document does not set max_header_bytes.
const (
	defaultPublicMaxHeaderBytes = 1 << 20   // 1 MiB, the net/http default made explicit
	defaultSystemMaxHeaderBytes = 256 << 10 // 256 KiB: probe and scrape requests are tiny
)

// DefaultRuntime returns the configuration in force for every setting a
// document leaves out. Load decodes the YAML on top of it, so a key that is
// present always wins, including an explicit zero (request_timeout: 0 turns
// the request deadline off rather than falling back to the default).
//
// A setting gets a default only when a safe, bounded value exists and its
// absence would otherwise be an error or an unbounded wait. The rest stay at
// their zero value on purpose:
//   - listeners.*.addr has no default: where to listen must be stated;
//   - listeners.*.tls is absent by default, which means plain HTTP;
//   - proxy.trusted_proxies is empty, which trusts no one;
//   - observability.tracing is off, and its endpoint is meaningless until it
//     is enabled;
//   - upstream_transport.max_conns_per_host is 0, which means no limit.
//
// The values match configs/aegis.yaml, so that file shows the defaults rather
// than overriding them.
func DefaultRuntime() Runtime {
	return Runtime{
		Listeners: Listeners{
			Public: HTTP{
				Timeouts: Timeouts{
					ReadTimeout:       10 * time.Second,
					ReadHeaderTimeout: 5 * time.Second,
					WriteTimeout:      30 * time.Second,
					IdleTimeout:       120 * time.Second,
					RequestTimeout:    30 * time.Second,
				},
				MaxHeaderBytes: defaultPublicMaxHeaderBytes,
			},
			System: HTTP{
				Timeouts: Timeouts{
					ReadTimeout:       5 * time.Second,
					ReadHeaderTimeout: 2 * time.Second,
					WriteTimeout:      10 * time.Second,
					IdleTimeout:       30 * time.Second,
				},
				MaxHeaderBytes: defaultSystemMaxHeaderBytes,
			},
		},
		UpstreamTransport: UpstreamTransport{
			MaxIdleConns:          1024,
			MaxIdleConnsPerHost:   1024,
			DialTimeout:           3 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
		Middleware: Middleware{
			Metrics:   true,
			RequestID: true,
		},
		Observability: Observability{
			Logging: Logging{
				Level:  "info",
				Format: "json",
			},
		},
	}
}
