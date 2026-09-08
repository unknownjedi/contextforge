package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/config"
	"github.com/your-org/contextforge/internal/logger"
	"github.com/your-org/contextforge/internal/queue"
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

	// 3. Initialize Asynq worker server
	workerServer, err := queue.NewWorkerServer(queue.WorkerConfig{
		RedisURL:    cfg.Redis.URL,
		Concurrency: 10,
	}, log)
	if err != nil {
		log.Fatal("failed to initialize worker server", zap.Error(err))
	}

	// 4. Handle graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-quit
		log.Info("shutting down worker server gracefully...", zap.String("signal", sig.String()))
		workerServer.Shutdown()
	}()

	// 5. Start worker processing loop
	if err := workerServer.Start(); err != nil {
		log.Fatal("worker server encountered fatal error", zap.Error(err))
	}

	log.Info("worker server stopped cleanly")
}
