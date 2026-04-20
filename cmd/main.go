package main

import (
	"context"
	"errors"
	"log"

	"github.com/aegis/internal/aegis"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p, err := aegis.New()
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	defer func() {
		if err = p.Close(); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	if err = p.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("runtime: %v", err)
	}
}
