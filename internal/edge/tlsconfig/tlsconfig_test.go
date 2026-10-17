package tlsconfig

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HAL-X9/aegis/internal/config"
)

// writeKeyPair writes a fresh self-signed certificate and its key as PEM
// files into dir and returns their paths.
func writeKeyPair(t *testing.T, dir, prefix string) (certPath, keyPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "aegis.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}

	certPath = filepath.Join(dir, prefix+".crt")
	keyPath = filepath.Join(dir, prefix+".key")
	writePEM(t, certPath, "CERTIFICATE", der)
	writePEM(t, keyPath, "EC PRIVATE KEY", keyDER)
	return certPath, keyPath
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	data := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestBuildNilMeansPlainHTTP(t *testing.T) {
	got, err := Build(nil)
	if err != nil || got != nil {
		t.Fatalf("Build(nil) = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestBuildLoadsCertificate(t *testing.T) {
	certPath, keyPath := writeKeyPair(t, t.TempDir(), "server")

	got, err := Build(&config.TLSConfig{CertFile: certPath, KeyFile: keyPath})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got.Certificates) != 1 {
		t.Errorf("certificates = %d, want 1", len(got.Certificates))
	}
	if got.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want TLS 1.2 (%#x)", got.MinVersion, tls.VersionTLS12)
	}
}

func TestBuildFailures(t *testing.T) {
	dir := t.TempDir()
	certA, keyA := writeKeyPair(t, dir, "a")
	_, keyB := writeKeyPair(t, dir, "b")
	missing := filepath.Join(dir, "absent.pem")

	tests := map[string]config.TLSConfig{
		"missing certificate": {CertFile: missing, KeyFile: keyA},
		"missing key":         {CertFile: certA, KeyFile: missing},
		"key of another pair": {CertFile: certA, KeyFile: keyB},
	}

	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Build(&cfg)
			if err == nil {
				t.Fatal("Build succeeded, want an error")
			}
			if !strings.Contains(err.Error(), cfg.CertFile) {
				t.Errorf("error = %v, want it to name the certificate path", err)
			}
		})
	}
}
