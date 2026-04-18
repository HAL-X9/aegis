package app

import (
	"fmt"
	"log"
	"net"

	auth "github.com/lootx/auth-service/api/gen/go/auth/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func NewGRPCServer(deps *Dependencies) *grpc.Server {
	server := grpc.NewServer()

	// Регистрируем health service для health checks
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Регистрируем сервис
	auth.RegisterAuthServiceServer(server, deps.AuthHandler)
	return server
}

func ListenGRPC(port int, grpcServer *grpc.Server) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}
	log.Printf("[INFO] gRPC server listening")
	return grpcServer.Serve(lis)
}
