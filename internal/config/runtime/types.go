package runtimeconfig

import (
	"crypto/tls"
	"time"
)

type HTTP struct {
	Addr           string      `yaml:"addr"`
	TLS            *tls.Config `yaml:"tls"`
	Timeouts       Timeouts    `yaml:"timeouts"`
	MaxHeaderBytes int         `yaml:"max_header_bytes"`
}

type Timeouts struct {
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
}

type Logging struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type Runtime struct {
	HTTP    HTTP    `yaml:"http"`
	Logging Logging `yaml:"logging"`
}
