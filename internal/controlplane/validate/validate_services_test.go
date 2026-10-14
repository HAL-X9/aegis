package validate

import (
	"strings"
	"testing"
	"time"

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
		{
			name: "timeouts within bounds",
			service: schema.Service{
				Upstream: schema.Upstream{Scheme: "http", Host: "localhost", Port: 8080},
				Timeout:  5 * time.Second,
				Retries:  schema.Retries{Attempts: 3, AttemptTimeout: 2 * time.Second},
			},
		},
		{
			name: "attempt timeout without service timeout",
			service: schema.Service{
				Upstream: schema.Upstream{Scheme: "http", Host: "localhost", Port: 8080},
				Retries:  schema.Retries{AttemptTimeout: time.Second},
			},
		},
		{
			name: "negative timeout",
			service: schema.Service{
				Upstream: schema.Upstream{Scheme: "http", Host: "localhost", Port: 8080},
				Timeout:  -time.Second,
			},
			wantErr:    true,
			wantSubstr: "timeout must not be negative",
		},
		{
			name: "negative attempt timeout",
			service: schema.Service{
				Upstream: schema.Upstream{Scheme: "http", Host: "localhost", Port: 8080},
				Retries:  schema.Retries{AttemptTimeout: -time.Second},
			},
			wantErr:    true,
			wantSubstr: "retries.attempt_timeout must not be negative",
		},
		{
			name: "attempt timeout exceeds timeout",
			service: schema.Service{
				Upstream: schema.Upstream{Scheme: "http", Host: "localhost", Port: 8080},
				Timeout:  time.Second,
				Retries:  schema.Retries{AttemptTimeout: 2 * time.Second},
			},
			wantErr:    true,
			wantSubstr: "retries.attempt_timeout must not exceed timeout",
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

func TestValidateServices_retryAttempts(t *testing.T) {
	t.Parallel()

	for _, attempts := range []int{-1, maxRetryAttempts + 1} {
		svc := testServices()["api"]
		svc.Retries.Attempts = attempts

		err := validateServices(schema.Services{"api": svc})

		assertErrorContains(t, err, "retries.attempts must be between 0 and 5")
	}

	for _, attempts := range []int{0, 1, maxRetryAttempts} {
		svc := testServices()["api"]
		svc.Retries.Attempts = attempts

		if err := validateServices(schema.Services{"api": svc}); err != nil {
			t.Fatalf("attempts=%d: unexpected error: %v", attempts, err)
		}
	}
}
