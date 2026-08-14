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
      host: mock-upstream
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

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

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
      host: localhost
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
