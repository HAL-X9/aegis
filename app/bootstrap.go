package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/lootx/auth-service/deployments/database/gen"
	"github.com/lootx/auth-service/internal/config"
	postgresrepo "github.com/lootx/auth-service/internal/infrastructure/adapters/postgres"
	redisrepo "github.com/lootx/auth-service/internal/infrastructure/adapters/redis"
	"github.com/lootx/auth-service/internal/infrastructure/clients"
	"github.com/lootx/auth-service/internal/infrastructure/crypto"
	"github.com/lootx/auth-service/internal/infrastructure/drivers/cache"
	"github.com/lootx/auth-service/internal/infrastructure/drivers/db"
	grpchandler "github.com/lootx/auth-service/internal/interfaces/grpc"
	"github.com/lootx/auth-service/internal/usecase"
	"github.com/lootx/auth-service/pkg/token"
)

type Dependencies struct {
	DB                *db.DB
	JWTManager        token.JWTManager
	EmailTokenManager *token.EmailTokenManager
	RabbitProducer    *clients.Producer
	AuthHandler       *grpchandler.AuthHandler
}

func NewDependencies(ctx context.Context, cfg config.Config) (*Dependencies, error) {
	// -------- Database --------- //
	database, err := db.NewDatabase(ctx, cfg.Database)
	if err != nil {
		log.Printf("[ERROR] Failed to initialize database: %v", err)
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// -------- Redis Client -------- //
	log.Printf("[INFO] Initializing Redis connection: addr=%s, db=%d, password=%v", cfg.Redis.Addr, cfg.Redis.DB, cfg.Redis.Password != "")
	redisClient, err := cache.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		log.Printf("[ERROR] Failed to initialize Redis: %v", err)
		return nil, fmt.Errorf("failed to initialize Redis: %w", err)
	}
	log.Printf("[INFO] Redis connection established successfully")

	// -------- UserStore -------- //
	userStore := redisrepo.NewUserStore(redisClient)

	// -------- Keys -------- //
	privateKey, publicKey, err := crypto.LoadRSAKeys(cfg.RSAKeys.PrivateKeyPath, cfg.RSAKeys.PublicKeyPath)
	if err != nil {
		log.Printf("[ERROR] Failed to load RSA keys from '%s' and '%s': %v", cfg.RSAKeys.PrivateKeyPath, cfg.RSAKeys.PublicKeyPath, err)
		return nil, fmt.Errorf("failed to load RSA keys: %w", err)
	}

	// -------- JWTManager -------- //
	jwtManager := token.NewKeyPair(privateKey, publicKey)

	// -------- EmailTokenManager ------- //
	emailTokenManager := token.NewEmailTokenManager(cfg.JWT.EmailSecret, 15*time.Minute)

	// -------- RabbitProducer -------- //
	rabbitProducer, err := clients.NewProducer(cfg.RabbitMQ.URL, cfg.RabbitMQ.Exchange, cfg.RabbitMQ.RoutingKey)
	if err != nil {
		log.Printf("[ERROR] Failed to initialize RabbitMQ producer (URL: %s, Exchange: %s, RoutingKey: %s): %v", cfg.RabbitMQ.URL, cfg.RabbitMQ.Exchange, cfg.RabbitMQ.RoutingKey, err)
		return nil, fmt.Errorf("failed to initialize RabbitMQ producer: %w", err)
	}

	// -------- Repository -------- //
	queries := gen.New(database.Pool)
	authRepository := postgresrepo.NewUserRepository(queries)

	// -------- Usecase -------- //
	authUsecase := usecase.NewAuthUsecase(
		authRepository,
		*rabbitProducer,
		emailTokenManager,
		jwtManager,
		&cfg.Captcha,
		&cfg.TokensTTL,
		&cfg.HMACSecret,
		&cfg.OTPConfig,
		userStore,
		database,
	)

	// -------- AuthHandler -------- //
	authHandler := grpchandler.NewAuthHandler(authUsecase, jwtManager)

	// ------- Return All Dependencies ------- //
	return &Dependencies{
		DB:                database,
		JWTManager:        jwtManager,
		EmailTokenManager: emailTokenManager,
		RabbitProducer:    rabbitProducer,
		AuthHandler:       authHandler,
	}, nil
}
