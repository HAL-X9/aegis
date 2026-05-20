package config

import (
	"fmt"
	"net"
	"time"
)

// Validate checks cfg for well-formed listen addresses, timeout ordering, and allowed enum values.
// cfg must not be nil.
func Validate(cfg *Runtime) error {
	if cfg == nil {
		return fmt.Errorf("validate runtime config: config is nil")
	}

	if err := validateListeners(&cfg.Listeners); err != nil {
		return fmt.Errorf("failed to validate listeners config: %w", err)
	}

	if err := validateUpstreamTransport(&cfg.UpstreamTransport); err != nil {
		return fmt.Errorf("failed validate upstream transport config: %w", err)
	}

	if err := validateLogging(&cfg.Observability.Logging); err != nil {
		return fmt.Errorf("failed validate logging config: %w", err)
	}

	return nil
}

// validateListeners checks listener configs for nil values and invalid HTTP settings.
func validateListeners(listeners *Listeners) error {
	if listeners == nil {
		return fmt.Errorf("listeners: configuration is nil")
	}

	if err := validateHTTP(&listeners.Public); err != nil {
		return fmt.Errorf("listeners.public: %w", err)
	}

	if err := validateHTTP(&listeners.System); err != nil {
		return fmt.Errorf("listeners.system: %w", err)
	}

	return nil
}

// validateHTTP enforces HTTP server field constraints used at listen time.
func validateHTTP(cfg *HTTP) error {
	if cfg == nil {
		return fmt.Errorf("http: configuration is nil")
	}

	if _, _, err := net.SplitHostPort(cfg.Addr); err != nil {
		return fmt.Errorf("invalid addr: %w", err)
	}

	if cfg.Timeouts.ReadTimeout < time.Millisecond {
		return fmt.Errorf("read_timeout must be at least 1ms")
	}

	if cfg.Timeouts.ReadHeaderTimeout > cfg.Timeouts.ReadTimeout {
		return fmt.Errorf("read_header_timeout cannot exceed read_timeout")
	}

	if cfg.Timeouts.WriteTimeout < 0 {
		return fmt.Errorf("write_timeout cannot be negative")
	}

	if cfg.MaxHeaderBytes < 0 {
		return fmt.Errorf("max_header_bytes cannot be negative")
	}

	return nil
}

// validateUpstreamTransport enforces non-negative limits and internal consistency of connection pool settings for outbound HTTP transport.
func validateUpstreamTransport(cfg *UpstreamTransport) error {
	if cfg.MaxIdleConns < 0 {
		return fmt.Errorf("max_idle_conns must be >= 0")
	}

	if cfg.MaxIdleConnsPerHost < 0 {
		return fmt.Errorf("max_idle_conns_per_host must be >= 0")
	}

	if cfg.MaxConnsPerHost < 0 {
		return fmt.Errorf("max_conns_per_host must be >= 0")
	}

	if cfg.MaxConnsPerHost > 0 && cfg.MaxIdleConnsPerHost > cfg.MaxConnsPerHost {
		return fmt.Errorf("max_idle_conns_per_host cannot exceed max_conns_per_host")
	}

	return nil
}

// validateLogging enforces logging level and format literals accepted by the logger wiring.
func validateLogging(cfg *Logging) error {
	if cfg == nil {
		return fmt.Errorf("logging: configuration is nil")
	}

	switch cfg.Level {
	case "debug", "info", "warn", "error", "dpanic", "panic", "fatal":
	default:
		return fmt.Errorf("logging_config.level must be one of: debug, info, warn, error, dpanic, panic, fatal")
	}

	switch cfg.Format {
	case "json", "console":
	default:
		return fmt.Errorf("logging_config.format must be %q or %q", "json", "console")
	}

	return nil
}
