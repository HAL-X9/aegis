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

func TestLoadReadsCertificate(t *testing.T) {
	certPath, keyPath := writeKeyPair(t, t.TempDir(), "server")

	got, err := Load(certPath, keyPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Certificates) != 1 {
		t.Errorf("certificates = %d, want 1", len(got.Certificates))
	}
	if got.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %#x, want TLS 1.2 (%#x)", got.MinVersion, tls.VersionTLS12)
	}
}

func TestLoadFailures(t *testing.T) {
	dir := t.TempDir()
	certA, keyA := writeKeyPair(t, dir, "a")
	_, keyB := writeKeyPair(t, dir, "b")
	missing := filepath.Join(dir, "absent.pem")

	tests := map[string]struct{ cert, key string }{
		"missing certificate": {cert: missing, key: keyA},
		"missing key":         {cert: certA, key: missing},
		"key of another pair": {cert: certA, key: keyB},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Load(tc.cert, tc.key)
			if err == nil {
				t.Fatal("Load succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tc.cert) {
				t.Errorf("error = %v, want it to name the certificate path", err)
			}
		})
	}
}
