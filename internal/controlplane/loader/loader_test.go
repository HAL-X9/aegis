package loader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Run("successfully loads valid config", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yaml")

		content := `
services:
  user-profile:
    upstream:
      scheme: http
      targets:
        - host: mock-upstream
          port: 8082

routes:
  - name: user-profile
    service: user-profile
    match:
      path_prefix: /api/v1/profile
      methods: [GET]
`

		err := os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		validated, err := Load(path)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		cfg := validated.Config()
		if cfg == nil {
			t.Fatalf("expected config, got nil")
		}

		if len(cfg.Services) != 1 {
			t.Fatalf("expected 1 service, got %d", len(cfg.Services))
		}

		if len(cfg.Routes) != 1 {
			t.Fatalf("expected 1 route, got %d", len(cfg.Routes))
		}

		if cfg.Routes[0].Service != "user-profile" {
			t.Fatalf(
				"expected route service %q, got %q",
				"user-profile",
				cfg.Routes[0].Service,
			)
		}
	})

	t.Run("keeps every upstream target in order", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		content := `
services:
  pool:
    upstream:
      scheme: http
      targets:
        - host: 10.0.0.2
          port: 8080
        - host: 10.0.0.1
          port: 8080

routes:
  - name: pool
    service: pool
    match:
      path_prefix: /
`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		validated, err := Load(path)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		targets := validated.Config().Services["pool"].Upstream.Targets
		if len(targets) != 2 || targets[0].Host != "10.0.0.2" || targets[1].Host != "10.0.0.1" {
			t.Fatalf("targets = %+v, want 10.0.0.2 then 10.0.0.1", targets)
		}
	})

	t.Run("rejects an upstream without targets", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		// The single-endpoint form that preceded upstream.targets.
		content := `
services:
  pool:
    upstream:
      scheme: http
      host: localhost
      port: 8080

routes:
  - name: pool
    service: pool
    match:
      path_prefix: /
`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		if _, err := Load(path); err == nil {
			t.Fatal("expected a decoding error for upstream.host")
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := Load("/non/existent/path.yaml")

		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("returns error for invalid config", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.yaml")

		content := `
services:
  user-profile:
    upstream:
      scheme: http
      targets:
        - host: localhost
          port: 8080

routes:
  - name: ""
    service: user-profile
    match:
      path_prefix: /test
`

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		_, err := Load(path)
		if err == nil {
			t.Fatal("expected validation error")
		}
	})
}
