package main

import (
	"context"
	"log"
	"os"
	"time"

	"deploy-service/pkg/deploypb"
	"repository-contorller-service-go/internal/controller"
	repohttp "repository-contorller-service-go/internal/http"
	"repository-contorller-service-go/internal/repository"
	"repository-contorller-service-go/internal/service"

	_ "repository-contorller-service-go/docs/swagger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// @title Repository Controller API
// @version 1.0
// @description API for authentication and user repository management.
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	jwtSecret := envOrDefault("REPO_CONTROL_JWT_SECRET", "local-dev-secret")
	addr := envOrDefault("REPO_CONTROL_ADDR", ":8080")
	deployGRPCAddr := envOrDefault("REPO_CONTROL_DEPLOY_GRPC_ADDR", "localhost:50051")
	deployAPIURL := envOrDefault("REPO_CONTROL_PUBLIC_URL", "http://localhost:8080")
	deployTimeout, err := envDurationOrDefault("REPO_CONTROL_DEPLOY_TIMEOUT", 10*time.Minute)
	if err != nil {
		log.Fatalf("parse deploy timeout: %v", err)
	}

	var store interface {
		repository.UserRepository
		repository.ClientRepositoryRepository
	}

	pgDSN := os.Getenv("REPO_CONTROL_POSTGRES_DSN")
	if pgDSN != "" {
		pg, err := repository.NewPostgresStore(context.Background(), pgDSN)
		if err != nil {
			log.Fatalf("init postgres store: %v", err)
		}
		defer pg.Close()
		log.Println("using PostgreSQL store")
		store = pg
	} else {
		dataPath := envOrDefault("REPO_CONTROL_DATA_PATH", "data/repositories.json")
		fs, err := repository.NewFileStore(dataPath)
		if err != nil {
			log.Fatalf("init file store: %v", err)
		}
		log.Println("using file store (set REPO_CONTROL_POSTGRES_DSN to use PostgreSQL)")
		store = fs
	}

	deployConn, err := grpc.NewClient(deployGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("init deploy grpc client: %v", err)
	}
	defer deployConn.Close()

	deployClient := deploypb.NewDeployServiceClient(deployConn)

	authService := service.NewAuthService(store, jwtSecret, 24*time.Hour)
	repoService := service.NewRepositoryService(store)
	deploymentService := service.NewDeploymentService(store, store, deployClient, deployTimeout, deployAPIURL)

	pollInterval, err := envDurationOrDefault("REPO_CONTROL_POLL_INTERVAL", 60*time.Second)
	if err != nil {
		log.Fatalf("parse poll interval: %v", err)
	}
	if pollInterval > 0 {
		poller := service.NewPoller(store, store, deploymentService, pollInterval)
		go poller.Start(context.Background())
	}

	webhookSecret := envOrDefault("REPO_CONTROL_WEBHOOK_SECRET", "")

	authController := controller.NewAuthController(authService)
	repoController := controller.NewRepositoryController(repoService, deploymentService)
	deploymentController := controller.NewDeploymentController(deploymentService)
	webhookController := controller.NewWebhookController(store, deploymentService, webhookSecret)

	router := repohttp.NewRouter(authController, repoController, deploymentController, webhookController, authService)

	log.Printf("repository controller service started on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("run repository controller service: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envDurationOrDefault(key string, fallback time.Duration) (time.Duration, error) {
	if value := os.Getenv(key); value != "" {
		return time.ParseDuration(value)
	}
	return fallback, nil
}
