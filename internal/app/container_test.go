package app

import (
	"net/http"
	"testing"
	"time"

	"github.com/HAL-X9/aegis/internal/config"
)

func TestNewHTTPServerMapsListenerSettings(t *testing.T) {
	listener := config.HTTP{
		Addr: "127.0.0.1:9090",
		Timeouts: config.Timeouts{
			ReadTimeout:       1 * time.Second,
			ReadHeaderTimeout: 2 * time.Second,
			WriteTimeout:      3 * time.Second,
			IdleTimeout:       4 * time.Second,
		},
		MaxHeaderBytes: 5,
	}
	handler := http.NewServeMux()

	server, err := newHTTPServer(listener, handler)
	if err != nil {
		t.Fatalf("newHTTPServer: %v", err)
	}

	if server.Addr != listener.Addr || server.Handler != http.Handler(handler) {
		t.Errorf("addr/handler = %q/%v, want %q/%v", server.Addr, server.Handler, listener.Addr, handler)
	}
	if server.ReadTimeout != listener.Timeouts.ReadTimeout ||
		server.ReadHeaderTimeout != listener.Timeouts.ReadHeaderTimeout ||
		server.WriteTimeout != listener.Timeouts.WriteTimeout ||
		server.IdleTimeout != listener.Timeouts.IdleTimeout {
		t.Errorf("timeouts = %v/%v/%v/%v, want %+v",
			server.ReadTimeout, server.ReadHeaderTimeout, server.WriteTimeout, server.IdleTimeout, listener.Timeouts)
	}
	if server.MaxHeaderBytes != listener.MaxHeaderBytes {
		t.Errorf("MaxHeaderBytes = %d, want %d", server.MaxHeaderBytes, listener.MaxHeaderBytes)
	}
	if server.TLSConfig != nil {
		t.Error("TLSConfig is set for a listener without a tls section, want plain HTTP")
	}
}

func TestNewHTTPServerReportsUnreadableCertificate(t *testing.T) {
	listener := config.HTTP{
		Addr: "127.0.0.1:0",
		TLS:  &config.TLSConfig{CertFile: "/nonexistent/c.pem", KeyFile: "/nonexistent/k.pem"},
	}

	if _, err := newHTTPServer(listener, http.NewServeMux()); err == nil {
		t.Fatal("newHTTPServer accepted an unreadable certificate, want an error")
	}
}
