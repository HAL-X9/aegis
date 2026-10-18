package app

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/HAL-X9/aegis/internal/observe/health"
)

// freeAddr returns a loopback address that was free a moment ago.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String()
}

func newServer(addr string) *http.Server {
	return &http.Server{Addr: addr, Handler: http.NewServeMux()}
}

func accepts(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// runAsync starts lifecycle.Run and returns a channel carrying its result.
func runAsync(ctx context.Context, lifecycle *Lifecycle) <-chan error {
	done := make(chan error, 1)
	go func() { done <- lifecycle.Run(ctx) }()
	return done
}

func awaitRun(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within 5s")
		return nil
	}
}

func TestNewLifecycleRejectsMissingParts(t *testing.T) {
	server := newServer("127.0.0.1:0")

	tests := map[string]*Dependencies{
		"nil dependencies": nil,
		"no public server": {SystemHTTP: server},
		"no system server": {PublicHTTP: server},
	}

	for name, deps := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := NewLifecycle(deps); err == nil {
				t.Fatal("NewLifecycle succeeded, want an error")
			}
		})
	}
}

func TestRunStopsBothListenersWhenContextIsCanceled(t *testing.T) {
	publicAddr, systemAddr := freeAddr(t), freeAddr(t)
	probe := health.NewHealth()

	lifecycle, err := NewLifecycle(&Dependencies{
		PublicHTTP: newServer(publicAddr),
		SystemHTTP: newServer(systemAddr),
		Health:     probe,
	})
	if err != nil {
		t.Fatalf("NewLifecycle: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := runAsync(ctx, lifecycle)

	waitUntil(t, "both listeners to accept", func() bool { return accepts(publicAddr) && accepts(systemAddr) })
	if err := probe.Readiness(); err != nil {
		t.Fatalf("readiness before shutdown: %v, want ready", err)
	}

	cancel()

	if err := awaitRun(t, done); err != nil {
		t.Fatalf("Run = %v, want nil after a requested shutdown", err)
	}
	if accepts(publicAddr) || accepts(systemAddr) {
		t.Error("a listener still accepts connections after Run returned")
	}
	if err := probe.Readiness(); err == nil {
		t.Error("readiness after shutdown = ready, want not ready")
	}
}

// TestRunFailsWhenAListenerCannotBind checks that one listener failing takes
// the whole process down instead of leaving the other one running.
func TestRunFailsWhenAListenerCannotBind(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy address: %v", err)
	}
	defer func() { _ = taken.Close() }()

	systemAddr := freeAddr(t)
	lifecycle, err := NewLifecycle(&Dependencies{
		PublicHTTP: newServer(taken.Addr().String()),
		SystemHTTP: newServer(systemAddr),
		Health:     health.NewHealth(),
	})
	if err != nil {
		t.Fatalf("NewLifecycle: %v", err)
	}

	err = awaitRun(t, runAsync(context.Background(), lifecycle))
	if err == nil {
		t.Fatal("Run succeeded although the public address was taken, want an error")
	}
	if !strings.Contains(err.Error(), "public") {
		t.Errorf("error = %v, want it to name the failing listener", err)
	}
	if accepts(systemAddr) {
		t.Error("the system listener is still running after the public one failed")
	}
}
