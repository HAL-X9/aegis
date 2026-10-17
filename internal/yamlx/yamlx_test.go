package yamlx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sample struct {
	Name  string `yaml:"name"`
	Count int    `yaml:"count"`
	On    bool   `yaml:"on"`
}

func writeFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}

func TestDecodeFile(t *testing.T) {
	got, err := DecodeFile[sample](writeFile(t, "name: a\ncount: 3\n"))
	if err != nil {
		t.Fatalf("DecodeFile: %v", err)
	}
	if want := (sample{Name: "a", Count: 3}); *got != want {
		t.Fatalf("DecodeFile = %+v, want %+v", *got, want)
	}
}

func TestDecodeFileRejectsUnknownKey(t *testing.T) {
	_, err := DecodeFile[sample](writeFile(t, "name: a\ncunt: 3\n"))
	if err == nil {
		t.Fatal("DecodeFile accepted an unknown key, want an error")
	}
	if !strings.Contains(err.Error(), "cunt") {
		t.Fatalf("error = %v, want it to name the unknown key", err)
	}
}

func TestDecodeFileReportsMissingFile(t *testing.T) {
	if _, err := DecodeFile[sample](filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("DecodeFile succeeded on a missing file, want an error")
	}
}

func TestDecodeFileWithDefaults(t *testing.T) {
	defaults := sample{Name: "d", Count: 7, On: true}

	got, err := DecodeFileWithDefaults(writeFile(t, "count: 0\n"), defaults)
	if err != nil {
		t.Fatalf("DecodeFileWithDefaults: %v", err)
	}

	want := sample{Name: "d", Count: 0, On: true}
	if *got != want {
		t.Fatalf("DecodeFileWithDefaults = %+v, want %+v (absent keys keep defaults, an explicit zero wins)", *got, want)
	}
}
