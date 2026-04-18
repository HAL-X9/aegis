package app

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/lootx/auth-service/internal/config"
)

func Run(ctx context.Context, cfg config.Config) error {
	deps, err := NewDependencies(ctx, cfg)
	if err != nil {
		return err
	}

	grpcServer := NewGRPCServer(deps)

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-stop
		log.Println("shutdown signal received, stopping server...")
		grpcServer.GracefulStop()
		log.Println("gRPC server stopped")
	}()

	log.Println("[INFO] gRPC server started")

	// Передаём сервер внутрь ListenGRPC — он сам вызывает Serve
	if err := ListenGRPC(cfg.GRPC.Port, grpcServer); err != nil {
		log.Println("gRPC server error")
		return err
	}

	return nil
}
