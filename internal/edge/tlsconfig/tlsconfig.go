// Package tlsconfig builds the crypto/tls configuration of an inbound
// listener from a certificate and key file. It reads from disk, which is why
// it lives at the edge and not in internal/config, a package of data and
// validation only.
package tlsconfig

import (
	"crypto/tls"
	"fmt"
)

// Load reads the static certificate and key at the given paths and returns the
// TLS configuration of a listener. The certificate is read once; it is not
// reloaded while the process runs.
func Load(certFile, keyFile string) (*tls.Config, error) {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load certificate %q and key %q: %w", certFile, keyFile, err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
