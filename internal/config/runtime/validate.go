package runtimeconfig

import (
	"fmt"
	"net"
	"time"
)

// Validate checks cfg for well-formed listen addresses, timeout ordering, and allowed enum values.
// cfg must not be nil.
func Validate(cfg *Runtime) error {
	if err := validateHTTP(&cfg.HTTP); err != nil {
		return fmt.Errorf("failed validate http config: %w", err)
	}

	if err := validateLogging(&cfg.Logging); err != nil {
		return fmt.Errorf("failed validate logging config: %w", err)
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
