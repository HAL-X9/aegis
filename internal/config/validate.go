package config

import (
	"errors"
	"fmt"
	"net"
	"time"
)

// Validate checks cfg for well-formed listen addresses, timeout ordering, and
// allowed enum values. It reports every violation at once, each prefixed with
// the YAML path of the offending field, joined with errors.Join.
//
// A rule that relates two fields runs only when both fields pass their own
// checks, so one bad value never produces a second, derived error.
func Validate(cfg *Runtime) error {
	if cfg == nil {
		return errors.New("config is nil")
	}

	var errs fieldErrors
	validateHTTP(&errs, "listeners.public", &cfg.Listeners.Public)
	validateHTTP(&errs, "listeners.system", &cfg.Listeners.System)
	validateUpstreamTransport(&errs, "upstream_transport", &cfg.UpstreamTransport)
	validateLogging(&errs, "observability.logging", &cfg.Observability.Logging)

	return errors.Join(errs...)
}

// fieldErrors collects rule violations, each bound to a YAML field path.
type fieldErrors []error

func (e *fieldErrors) add(path, format string, args ...any) {
	*e = append(*e, fmt.Errorf("%s: %s", path, fmt.Sprintf(format, args...)))
}

// validateHTTP enforces HTTP server field constraints used at listen time.
func validateHTTP(errs *fieldErrors, path string, cfg *HTTP) {
	if _, _, err := net.SplitHostPort(cfg.Addr); err != nil {
		errs.add(path+".addr", "%v", err)
	}

	t := cfg.Timeouts
	readOK := t.ReadTimeout >= time.Millisecond
	if !readOK {
		errs.add(path+".timeouts.read_timeout", "must be at least 1ms")
	}
	if readOK && t.ReadHeaderTimeout > t.ReadTimeout {
		errs.add(path+".timeouts.read_header_timeout", "cannot exceed read_timeout")
	}
	if t.WriteTimeout < 0 {
		errs.add(path+".timeouts.write_timeout", "cannot be negative")
	}

	if cfg.MaxHeaderBytes < 0 {
		errs.add(path+".max_header_bytes", "cannot be negative")
	}

	if cfg.TLS != nil {
		if cfg.TLS.CertFile == "" {
			errs.add(path+".tls.cert_file", "is required")
		}
		if cfg.TLS.KeyFile == "" {
			errs.add(path+".tls.key_file", "is required")
		}
	}
}

// validateUpstreamTransport enforces non-negative limits and internal
// consistency of connection pool settings for outbound HTTP transport.
func validateUpstreamTransport(errs *fieldErrors, path string, cfg *UpstreamTransport) {
	if cfg.MaxIdleConns < 0 {
		errs.add(path+".max_idle_conns", "must be >= 0")
	}

	// Zero is rejected rather than passed through: net/http treats it as 2,
	// which forces a new upstream connection for most concurrent requests.
	if cfg.MaxIdleConnsPerHost < 1 {
		errs.add(path+".max_idle_conns_per_host", "must be >= 1")
	}
	if cfg.MaxConnsPerHost < 0 {
		errs.add(path+".max_conns_per_host", "must be >= 0")
	}

	// A positive max_conns_per_host is itself valid, and max_idle_conns_per_host
	// below 1 cannot exceed it, so this rule never repeats an error above.
	// Zero means no limit.
	if cfg.MaxConnsPerHost > 0 && cfg.MaxIdleConnsPerHost > cfg.MaxConnsPerHost {
		errs.add(path+".max_idle_conns_per_host", "cannot exceed max_conns_per_host")
	}
}

// validateLogging enforces logging level and format literals accepted by the logger wiring.
func validateLogging(errs *fieldErrors, path string, cfg *Logging) {
	switch cfg.Level {
	case "debug", "info", "warn", "error", "dpanic", "panic", "fatal":
	default:
		errs.add(path+".level", "must be one of: debug, info, warn, error, dpanic, panic, fatal; got %q", cfg.Level)
	}

	switch cfg.Format {
	case "json", "console":
	default:
		errs.add(path+".format", "must be %q or %q; got %q", "json", "console", cfg.Format)
	}
}
