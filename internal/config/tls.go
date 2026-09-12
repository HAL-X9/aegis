package config

import (
	"crypto/tls"
	"fmt"
)

// BuildTLSConfig loads the static certificate configured for an inbound
// listener. A nil TLS section deliberately means plain HTTP.
func BuildTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	if cfg == nil {
		return nil, nil
	}

	certificate, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load certificate %q and key %q: %w", cfg.CertFile, cfg.KeyFile, err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{certificate},
	}, nil
}
