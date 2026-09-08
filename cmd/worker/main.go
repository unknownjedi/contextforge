package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/chunk"
	"github.com/your-org/contextforge/internal/config"
	"github.com/your-org/contextforge/internal/connector"
	"github.com/your-org/contextforge/internal/crypto"
	"github.com/your-org/contextforge/internal/database"
	"github.com/your-org/contextforge/internal/ingest"
	"github.com/your-org/contextforge/internal/logger"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/queue"
	"github.com/your-org/contextforge/internal/repository"
	"github.com/your-org/contextforge/internal/worker"
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

	log.Info("starting ContextForge background worker server",
		zap.String("redis_url", cfg.Redis.URL),
		zap.Int("concurrency", 10),
	)

	// 3. Initialize database pool
	db, err := database.New(&cfg.Database, log)
	if err != nil {
		log.Fatal("failed to initialize database pool", zap.Error(err))
	}
	defer func() { _ = db.Close() }()

	// 4. Initialize repositories
	docRepo := repository.NewDocumentRepository(db.EntClient)
	jobRepo := repository.NewJobRepository(db.EntClient)
	vectorRepo := repository.NewPgVectorRepository(db.SQLDB)

	// 5. Initialize embedder
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

	// 6. Initialize ingestion pipeline
	pipeline := worker.NewIngestionPipeline(
		docRepo,
		vectorRepo,
		jobRepo,
		embedder,
		chunk.NewChunker(chunk.DefaultOptions()),
		ingest.NewMemoryDedupCache(),
		nil,
		log,
	)

	// 7. Initialize database ingestion pipeline
	encKey, err := crypto.KeyFromHex(cfg.Auth.TokenEncryptionKey)
	if err != nil {
		log.Fatal("invalid token encryption key", zap.Error(err))
	}
	dbPipeline := worker.NewDatabaseIngestionPipeline(
		db.EntClient,
		connector.DefaultRegistry(),
		encKey,
		docRepo,
		vectorRepo,
		jobRepo,
		embedder,
		chunk.NewChunker(chunk.DefaultOptions()),
		log,
	)

	// 8. Initialize Asynq worker server
	workerServer, err := queue.NewWorkerServer(queue.WorkerConfig{
		RedisURL:    cfg.Redis.URL,
		Concurrency: 10,
	}, log)
	if err != nil {
		log.Fatal("failed to initialize worker server", zap.Error(err))
	}

	// Register task handlers
	workerServer.RegisterHandler(queue.TypeRepoSync, pipeline.ProcessSyncTask)
	workerServer.RegisterHandler(queue.TypeDatabaseSync, dbPipeline.ProcessDatabaseSyncTask)

	// 8. Handle graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-quit
		log.Info("shutting down worker server gracefully...", zap.String("signal", sig.String()))
		workerServer.Shutdown()
	}()

	// 9. Start worker processing loop
	if err := workerServer.Start(); err != nil {
		log.Fatal("worker server encountered fatal error", zap.Error(err))
	}

	log.Info("worker server stopped cleanly")
}
