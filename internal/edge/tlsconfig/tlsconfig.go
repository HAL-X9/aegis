// Package tlsconfig builds the crypto/tls configuration of an inbound
// listener from its settings. It reads the certificate and key from disk, which
// is why it lives at the edge and not in internal/config, a package of data
// and validation only.
package tlsconfig

import (
	"crypto/tls"
	"fmt"

	"github.com/HAL-X9/aegis/internal/config"
)

// Build loads the static certificate configured for an inbound listener. A
// nil cfg deliberately means plain HTTP and yields a nil *tls.Config.
//
// The certificate is read once; it is not reloaded while the process runs.
func Build(cfg *config.TLSConfig) (*tls.Config, error) {
	if cfg == nil {
		return nil, nil
	}

	certificate, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf(
			"load certificate %q and key %q: %w",
			cfg.CertFile,
			cfg.KeyFile,
			err,
		)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
	}, nil
}
