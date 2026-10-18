package config

import (
	"strings"
	"testing"
	"time"
)

// validRuntime returns DefaultRuntime completed with the two listen addresses,
// the only settings that have no default.
func validRuntime() *Runtime {
	cfg := DefaultRuntime()
	cfg.Listeners.Public.Addr = ":8080"
	cfg.Listeners.System.Addr = "127.0.0.1:18080"
	return &cfg
}

func TestValidateAcceptsDefaultsWithAddresses(t *testing.T) {
	if err := Validate(validRuntime()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateRejectsNilConfig(t *testing.T) {
	if err := Validate(nil); err == nil {
		t.Fatal("Validate(nil) succeeded, want an error")
	}
}

func TestValidateRules(t *testing.T) {
	tests := map[string]struct {
		mutate  func(*Runtime)
		wantErr string
	}{
		"public address without a port": {
			mutate:  func(c *Runtime) { c.Listeners.Public.Addr = "localhost" },
			wantErr: "listeners.public",
		},
		"system address empty": {
			mutate:  func(c *Runtime) { c.Listeners.System.Addr = "" },
			wantErr: "listeners.system",
		},
		"read_timeout below 1ms": {
			mutate:  func(c *Runtime) { c.Listeners.Public.Timeouts.ReadTimeout = 0 },
			wantErr: "read_timeout must be at least 1ms",
		},
		"read_header_timeout above read_timeout": {
			mutate: func(c *Runtime) {
				c.Listeners.Public.Timeouts.ReadTimeout = time.Second
				c.Listeners.Public.Timeouts.ReadHeaderTimeout = 2 * time.Second
			},
			wantErr: "read_header_timeout cannot exceed read_timeout",
		},
		"negative write_timeout": {
			mutate:  func(c *Runtime) { c.Listeners.System.Timeouts.WriteTimeout = -time.Second },
			wantErr: "write_timeout cannot be negative",
		},
		"negative max_header_bytes": {
			mutate:  func(c *Runtime) { c.Listeners.Public.MaxHeaderBytes = -1 },
			wantErr: "max_header_bytes cannot be negative",
		},
		"tls without cert_file": {
			mutate:  func(c *Runtime) { c.Listeners.Public.TLS = &TLSConfig{KeyFile: "k"} },
			wantErr: "cert_file is required",
		},
		"tls without key_file": {
			mutate:  func(c *Runtime) { c.Listeners.Public.TLS = &TLSConfig{CertFile: "c"} },
			wantErr: "key_file is required",
		},
		"negative max_idle_conns": {
			mutate:  func(c *Runtime) { c.UpstreamTransport.MaxIdleConns = -1 },
			wantErr: "max_idle_conns must be >= 0",
		},
		"zero max_idle_conns_per_host": {
			mutate:  func(c *Runtime) { c.UpstreamTransport.MaxIdleConnsPerHost = 0 },
			wantErr: "max_idle_conns_per_host must be >= 1",
		},
		"negative max_conns_per_host": {
			mutate:  func(c *Runtime) { c.UpstreamTransport.MaxConnsPerHost = -1 },
			wantErr: "max_conns_per_host must be >= 0",
		},
		"idle per host above the connection cap": {
			mutate: func(c *Runtime) {
				c.UpstreamTransport.MaxConnsPerHost = 10
				c.UpstreamTransport.MaxIdleConnsPerHost = 11
			},
			wantErr: "cannot exceed max_conns_per_host",
		},
		"unknown log level": {
			mutate:  func(c *Runtime) { c.Observability.Logging.Level = "verbose" },
			wantErr: "level must be one of",
		},
		"unknown log format": {
			mutate:  func(c *Runtime) { c.Observability.Logging.Format = "xml" },
			wantErr: "format must be",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validRuntime()
			tc.mutate(cfg)

			err := Validate(cfg)
			if err == nil {
				t.Fatal("Validate succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateAllowsUnlimitedConnectionsPerHost(t *testing.T) {
	cfg := validRuntime()
	cfg.UpstreamTransport.MaxConnsPerHost = 0
	cfg.UpstreamTransport.MaxIdleConnsPerHost = 5000

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate: %v, want max_conns_per_host: 0 to mean no limit", err)
	}
}
