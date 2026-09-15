package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
)

// httpComponent adapts an *http.Server to the run/shutdown lifecycle used
// by Lifecycle.
//
// It intentionally carries no mutex, no sync.Once, and no "already closed"
// flag: Lifecycle is the only caller, and Lifecycle's own structure already
// guarantees run is invoked exactly once and shutdown at most once per
// component per process. Adding defensive synchronization here would
// protect against a misuse pattern that cannot occur, at the cost of
// hiding that guarantee behind a lock instead of stating it in the type
// that actually owns it.
type httpComponent struct {
	name   string
	server *http.Server
}

func newHTTPComponent(name string, server *http.Server) (*httpComponent, error) {
	if server == nil {
		return nil, fmt.Errorf("%s: http server is nil", name)
	}
	return &httpComponent{name: name, server: server}, nil
}

// run binds the listener and blocks serving traffic until shutdown stops
// the server (or the bind/serve fails outright). It returns nil for the
// expected case — Shutdown was called elsewhere, Serve consequently
// returned http.ErrServerClosed — and a wrapped error for anything else.
func (c *httpComponent) run() error {
	scheme := "http"
	if c.server.TLSConfig != nil {
		scheme = "https"
	}
	log.Printf("aegis: %s listener starting on %s (%s)", c.name, c.server.Addr, scheme)

	ln, err := net.Listen("tcp", c.server.Addr)
	if err != nil {
		return fmt.Errorf("%s: listen on %s: %w", c.name, c.server.Addr, err)
	}
	if c.server.TLSConfig != nil {
		ln = tls.NewListener(ln, c.server.TLSConfig)
	}

	if err := c.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%s: serve: %w", c.name, err)
	}
	return nil
}

// shutdown stops accepting new connections and waits for in-flight
// requests to finish, bounded by ctx.
func (c *httpComponent) shutdown(ctx context.Context) error {
	err := c.server.Shutdown(ctx)
	if err != nil {
		_ = c.server.Close()
		return fmt.Errorf("%s: shutdown: %w", c.name, err)
	}
	log.Printf("aegis: %s listener stopped", c.name)
	return nil
}
