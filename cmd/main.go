package main

import (
	"context"
	"errors"
	"log"

	"github.com/aegis/internal/app"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p, err := app.New()
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	defer func() {
		if err = p.Close(); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	if err = p.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("app: %v", err)
	}
}
