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
routes:
  - name: test
    match:
      path_prefix: /test
      methods: ["GET"]
    upstream:
      scheme: http
      host: localhost
      port: 8080
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

		if len(cfg.Routes) != 1 {
			t.Fatalf("expected 1 route, got %d", len(cfg.Routes))
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := Load("/non/existent/path.yaml")

		if err == nil {
			t.Fatalf("expected error for missing file")
		}
	})

	t.Run("returns error for invalid config", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.yaml")

		content := `
routes:
  - name: ""
`

		_ = os.WriteFile(path, []byte(content), 0644)

		_, err := Load(path)
		if err == nil {
			t.Fatalf("expected validation error")
		}
	})
}
