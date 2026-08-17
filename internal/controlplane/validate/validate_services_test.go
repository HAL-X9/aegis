package validate

import (
	"strings"
	"testing"

	"github.com/HAL-X9/aegis/internal/controlplane/schema"
)

func TestValidateServices(t *testing.T) {
	t.Parallel()

	t.Run("nil services", func(t *testing.T) {
		t.Parallel()

		if err := validateServices(nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty services", func(t *testing.T) {
		t.Parallel()

		if err := validateServices(schema.Services{}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid services", func(t *testing.T) {
		t.Parallel()

		services := schema.Services{
			"user-profile": {
				Upstream: schema.Upstream{
					Scheme: "http",
					Host:   "mock-upstream",
					Port:   8082,
				},
			},
			"billing": {
				Upstream: schema.Upstream{
					Scheme: "https",
					Host:   "billing.internal",
					Port:   443,
				},
			},
		}

		if err := validateServices(services); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("invalid service includes service name", func(t *testing.T) {
		t.Parallel()

		services := schema.Services{
			"user-profile": {
				Upstream: schema.Upstream{
					Scheme: "grpc",
					Host:   "mock-upstream",
					Port:   8082,
				},
			},
		}

		err := validateServices(services)
		if err == nil {
			t.Fatal("expected error")
		}

		if !strings.Contains(err.Error(), `invalid service "user-profile"`) {
			t.Fatalf("expected service name in error, got: %v", err)
		}

		if !strings.Contains(err.Error(), "upstream.scheme must be one of: http, https") {
			t.Fatalf("expected validation error, got: %v", err)
		}
	})
}

func TestValidateService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		service    schema.Service
		wantErr    bool
		wantSubstr string
	}{
		{
			name: "valid http",
			service: schema.Service{
				Upstream: schema.Upstream{
					Scheme: "http",
					Host:   "localhost",
					Port:   8080,
				},
			},
		},
		{
			name: "valid https",
			service: schema.Service{
				Upstream: schema.Upstream{
					Scheme: "https",
					Host:   "example.com",
					Port:   443,
				},
			},
		},
		{
			name: "empty scheme",
			service: schema.Service{
				Upstream: schema.Upstream{
					Host: "localhost",
					Port: 8080,
				},
			},
			wantErr:    true,
			wantSubstr: "upstream.scheme is required and must be non-empty",
		},
		{
			name: "blank scheme",
			service: schema.Service{
				Upstream: schema.Upstream{
					Scheme: "   ",
					Host:   "localhost",
					Port:   8080,
				},
			},
			wantErr:    true,
			wantSubstr: "upstream.scheme is required and must be non-empty",
		},
		{
			name: "unsupported scheme",
			service: schema.Service{
				Upstream: schema.Upstream{
					Scheme: "grpc",
					Host:   "localhost",
					Port:   8080,
				},
			},
			wantErr:    true,
			wantSubstr: "upstream.scheme must be one of: http, https",
		},
		{
			name: "empty host",
			service: schema.Service{
				Upstream: schema.Upstream{
					Scheme: "http",
					Port:   8080,
				},
			},
			wantErr:    true,
			wantSubstr: "upstream.host is required and must not be blank",
		},
		{
			name: "blank host",
			service: schema.Service{
				Upstream: schema.Upstream{
					Scheme: "http",
					Host:   "   ",
					Port:   8080,
				},
			},
			wantErr:    true,
			wantSubstr: "upstream.host is required and must not be blank",
		},
		{
			name: "port zero",
			service: schema.Service{
				Upstream: schema.Upstream{
					Scheme: "http",
					Host:   "localhost",
					Port:   0,
				},
			},
			wantErr:    true,
			wantSubstr: "upstream.port must be in range 1..65535",
		},
		{
			name: "negative port",
			service: schema.Service{
				Upstream: schema.Upstream{
					Scheme: "http",
					Host:   "localhost",
					Port:   -1,
				},
			},
			wantErr:    true,
			wantSubstr: "upstream.port must be in range 1..65535",
		},
		{
			name: "port above maximum",
			service: schema.Service{
				Upstream: schema.Upstream{
					Scheme: "http",
					Host:   "localhost",
					Port:   65536,
				},
			},
			wantErr:    true,
			wantSubstr: "upstream.port must be in range 1..65535",
		},
		{
			name: "minimum valid port",
			service: schema.Service{
				Upstream: schema.Upstream{
					Scheme: "http",
					Host:   "localhost",
					Port:   1,
				},
			},
		},
		{
			name: "maximum valid port",
			service: schema.Service{
				Upstream: schema.Upstream{
					Scheme: "http",
					Host:   "localhost",
					Port:   65535,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateService(tt.service)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}

				if !strings.Contains(err.Error(), tt.wantSubstr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantSubstr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
