package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// minimalRuntimeYAML is the smallest valid document: only the listen
// addresses have no default. extra is appended verbatim.
func minimalRuntimeYAML(extra string) string {
	return `
listeners:
  public:
    addr: ":8080"
  system:
    addr: "127.0.0.1:18080"
` + extra
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aegis.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadMiddleware(t *testing.T) {
	tests := map[string]struct {
		extra string
		want  Middleware
	}{
		"section omitted enables everything": {
			extra: "",
			want:  Middleware{Metrics: true, RequestID: true},
		},
		"empty section enables everything": {
			extra: "middleware: {}\n",
			want:  Middleware{Metrics: true, RequestID: true},
		},
		"both explicitly enabled": {
			extra: "middleware:\n  metrics: true\n  request_id: true\n",
			want:  Middleware{Metrics: true, RequestID: true},
		},
		"metrics disabled": {
			extra: "middleware:\n  metrics: false\n",
			want:  Middleware{Metrics: false, RequestID: true},
		},
		"request_id disabled": {
			extra: "middleware:\n  request_id: false\n",
			want:  Middleware{Metrics: true, RequestID: false},
		},
		"both disabled": {
			extra: "middleware:\n  metrics: false\n  request_id: false\n",
			want:  Middleware{Metrics: false, RequestID: false},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, err := Load(writeConfig(t, minimalRuntimeYAML(tc.extra)))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.Middleware != tc.want {
				t.Fatalf("Middleware = %+v, want %+v", cfg.Middleware, tc.want)
			}
		})
	}
}

func TestLoadRejectsUnknownMiddlewareKey(t *testing.T) {
	_, err := Load(writeConfig(t, minimalRuntimeYAML("middleware:\n  request-id: false\n")))
	if err == nil {
		t.Fatal("Load accepted an unknown key under middleware, want an error")
	}
	if !strings.Contains(err.Error(), "request-id") {
		t.Fatalf("error = %v, want it to name the unknown key", err)
	}
}

func TestLoadMinimalDocumentYieldsDefaults(t *testing.T) {
	cfg, err := Load(writeConfig(t, minimalRuntimeYAML("")))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	want := DefaultRuntime()
	want.Listeners.Public.Addr = ":8080"
	want.Listeners.System.Addr = "127.0.0.1:18080"

	if !reflect.DeepEqual(*cfg, want) {
		t.Fatalf("Load = %+v, want DefaultRuntime plus the two addresses: %+v", *cfg, want)
	}
}

func TestLoadKeepsDefaultsAroundOverriddenKeys(t *testing.T) {
	cfg, err := Load(writeConfig(t, `
listeners:
  public:
    addr: ":8080"
    timeouts:
      read_timeout: 7s
      request_timeout: 0s
  system:
    addr: "127.0.0.1:18080"
upstream_transport:
  dial_timeout: 1s
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	def := DefaultRuntime()
	public := cfg.Listeners.Public.Timeouts

	if public.ReadTimeout != 7*time.Second {
		t.Errorf("read_timeout = %v, want the document's 7s", public.ReadTimeout)
	}
	if public.RequestTimeout != 0 {
		t.Errorf("request_timeout = %v, want the explicit 0 to beat the default", public.RequestTimeout)
	}
	if public.WriteTimeout != def.Listeners.Public.Timeouts.WriteTimeout {
		t.Errorf("write_timeout = %v, want the default %v", public.WriteTimeout, def.Listeners.Public.Timeouts.WriteTimeout)
	}
	if cfg.UpstreamTransport.DialTimeout != time.Second {
		t.Errorf("dial_timeout = %v, want the document's 1s", cfg.UpstreamTransport.DialTimeout)
	}
	if cfg.UpstreamTransport.ResponseHeaderTimeout != def.UpstreamTransport.ResponseHeaderTimeout {
		t.Errorf("response_header_timeout = %v, want the default %v",
			cfg.UpstreamTransport.ResponseHeaderTimeout, def.UpstreamTransport.ResponseHeaderTimeout)
	}
}

func TestLoadStillRequiresListenAddresses(t *testing.T) {
	if _, err := Load(writeConfig(t, "middleware: {}\n")); err == nil {
		t.Fatal("Load accepted a document without listen addresses, want an error")
	}
}
