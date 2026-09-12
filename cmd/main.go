package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/HAL-X9/aegis/internal/app"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("aegis: %v", err)
	}
}

// run holds everything main would otherwise do directly, and returns an
// error instead of calling os.Exit itself. log.Fatalf (called only here,
// once, in main) exits the process without running deferred calls — so
// nothing that needs cleanup may live downstream of it. Keeping that call
// in main, and nowhere else, is what makes that safe.
func run() error {
	// Profiling knobs are process-global. Set them before any goroutine
	// that could contend on a mutex or block on a channel starts — that
	// means before app.Run, which is what actually starts the listeners.
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a, err := app.New()
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	// Run blocks until ctx is canceled (SIGINT/SIGTERM above) or a listener
	// fails, and does not return until every listener it started has also
	// stopped. There is nothing left to close here afterward.
	if err = a.Run(ctx); err != nil {
		return fmt.Errorf("run: %w", err)
	}
	return nil
}
