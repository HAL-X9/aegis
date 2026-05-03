package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type ShutdownManager struct {
	timeout time.Duration
}

func NewShutdownManager(timeout time.Duration) *ShutdownManager {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &ShutdownManager{timeout: timeout}
}

func (s *ShutdownManager) Wait(parent context.Context) <-chan struct{} {
	done := make(chan struct{})

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		defer close(done)
		defer signal.Stop(sigCh)
		select {
		case <-parent.Done():
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGINT:
				_, _ = fmt.Fprintln(os.Stderr, "\nInterrupt received, shutting down...")
			case syscall.SIGTERM:
				_, _ = fmt.Fprintln(os.Stderr, "\nTerminated, shutting down...")
			default:
				_, _ = fmt.Fprintf(os.Stderr, "\nSignal %v, shutting down...\n", sig)
			}
		}
	}()

	return done
}
