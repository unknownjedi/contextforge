package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/api"
	"github.com/your-org/contextforge/internal/api/handler"
	"github.com/your-org/contextforge/internal/config"
	"github.com/your-org/contextforge/internal/database"
	"github.com/your-org/contextforge/internal/logger"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/queue"
	"github.com/your-org/contextforge/internal/repository"
	"github.com/your-org/contextforge/internal/retrieval"
	"github.com/your-org/contextforge/internal/service"
)

func main() {
	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Initialize structured logger
	log, err := logger.New(cfg.Server.Env, "info")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	log.Info("starting ContextForge API server",
		zap.Int("port", cfg.Server.Port),
		zap.String("env", cfg.Server.Env),
	)

	// 3. Initialize database pool
	db, err := database.New(&cfg.Database, log)
	if err != nil {
		log.Fatal("failed to initialize database pool", zap.Error(err))
	}
	defer func() { _ = db.Close() }()

	// 4. Initialize Redis queue client (optional in standalone dev)
	var queueClient *queue.Client
	if cfg.Redis.URL != "" {
		qc, qerr := queue.NewClient(cfg.Redis.URL)
		if qerr != nil {
			log.Warn("could not connect to Redis queue; background sync tasks will not be queued", zap.Error(qerr))
		} else {
			queueClient = qc
			defer func() { _ = queueClient.Close() }()
		}
	}

	// 5. Initialize Repositories
	userRepo := repository.NewUserRepository(db.EntClient)
	projectRepo := repository.NewProjectRepository(db.EntClient)
	sourceRepo := repository.NewSourceRepository(db.EntClient)
	docRepo := repository.NewDocumentRepository(db.EntClient)
	jobRepo := repository.NewJobRepository(db.EntClient)
	vectorRepo := repository.NewPgVectorRepository(db.SQLDB)

	// 6. Initialize Services
	authService, err := service.NewAuthService(
		userRepo,
		cfg.Auth.TokenEncryptionKey,
		cfg.Auth.JWTSecret,
		cfg.Auth.SessionExpiry,
		nil,
	)
	if err != nil {
		log.Fatal("failed to initialize auth service", zap.Error(err))
	}

	// Initialize AI providers
	embedder, err := provider.NewEmbeddingProvider(provider.FactoryConfig{
		Type:           cfg.Providers.Defaults.Embedding,
		APIKey:         cfg.Providers.APIKeys.OpenAI,
		EmbeddingModel: "text-embedding-3-small",
		Dimension:      1536,
	})
	if err != nil {
		log.Warn("falling back to mock embedding provider", zap.Error(err))
		embedder = provider.NewMockEmbeddingProvider(768)
	}

	llm, err := provider.NewLLMProvider(provider.FactoryConfig{
		Type:    cfg.Providers.Defaults.LLM,
		APIKey:  cfg.Providers.APIKeys.OpenAI,
		Model:   "gpt-4o",
		Timeout: 60 * time.Second,
	})
	if err != nil {
		log.Warn("falling back to mock LLM provider", zap.Error(err))
		llm = provider.NewMockLLMProvider("ContextForge AI: ready to answer code questions.")
	}

	hybridRetriever := retrieval.NewHybridRetriever(vectorRepo, embedder)
	ragService := service.NewRAGService(hybridRetriever, llm)

	// 7. Initialize Handlers
	authH := handler.NewAuthHandler(authService)
	projectH := handler.NewProjectHandler(projectRepo, log)
	sourceH := handler.NewSourceHandler(sourceRepo, jobRepo, queueClient, log)
	docH := handler.NewDocumentHandler(docRepo, vectorRepo, log)
	jobH := handler.NewJobHandler(jobRepo, log)
	chatH := handler.NewChatHandler(ragService, log)
	webhookH := handler.NewWebhookHandler(cfg.Auth.WebhookSecret, log)

	// 8. Initialize server router & dependency checkers
	server := api.NewServer(cfg, log, db, nil)
	server.MountRoutes(api.Handlers{
		Auth:     authH,
		Project:  projectH,
		Source:   sourceH,
		Document: docH,
		Job:      jobH,
		Chat:     chatH,
		Webhook:  webhookH,
	}, projectRepo)

	httpServer := server.HTTPServer()

	// 9. Start HTTP listener in background goroutine
	go func() {
		log.Info("listening for HTTP requests", zap.String("addr", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	// 10. Graceful shutdown on SIGINT or SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	sig := <-quit

	log.Info("shutting down server...", zap.String("signal", sig.String()))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("server forced to shutdown", zap.Error(err))
	} else {
		log.Info("server gracefully stopped")
	}

	server.Close()
	if queueClient != nil {
		_ = queueClient.Close()
	}
	if err := db.Close(); err != nil {
		log.Error("error closing database", zap.Error(err))
	}
}
