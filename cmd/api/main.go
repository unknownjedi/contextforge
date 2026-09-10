package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/api"
	"github.com/your-org/contextforge/internal/api/handler"
	"github.com/your-org/contextforge/internal/chunk"
	"github.com/your-org/contextforge/internal/config"
	"github.com/your-org/contextforge/internal/connector"
	"github.com/your-org/contextforge/internal/crypto"
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
	conversationRepo := repository.NewConversationRepository(db.EntClient)
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

	defaultLLMType := cfg.Providers.Defaults.LLM
	var defaultAPIKey string
	switch defaultLLMType {
	case "anthropic":
		defaultAPIKey = cfg.Providers.APIKeys.Anthropic
	case "gemini":
		defaultAPIKey = cfg.Providers.APIKeys.Gemini
	default:
		defaultAPIKey = cfg.Providers.APIKeys.OpenAI
	}

	var llm provider.LLMProvider
	if defaultLLMType == "cli_opencode" || defaultLLMType == "opencode" {
		binPath := cfg.Providers.CLIPaths.OpenCodeCLI
		if binPath == "" {
			binPath = "opencode"
		}
		if _, err := exec.LookPath(binPath); err != nil {
			log.Info("opencode CLI binary not found in PATH; using local Ollama LLM provider", zap.String("bin", binPath))
			llm = provider.NewOllamaLLMProvider("http://localhost:11434", "qwen3.5:latest", 90*time.Second)
		}
	}

	if llm == nil {
		var lErr error
		llm, lErr = provider.NewLLMProvider(provider.FactoryConfig{
			Type:       defaultLLMType,
			APIKey:     defaultAPIKey,
			Model:      "gpt-4o",
			BinaryPath: cfg.Providers.CLIPaths.OpenCodeCLI,
			Timeout:    60 * time.Second,
		})
		if lErr != nil {
			log.Warn("falling back to Ollama / mock LLM provider", zap.Error(lErr))
			llm = provider.NewOllamaLLMProvider("http://localhost:11434", "qwen3.5:latest", 90*time.Second)
		}
	}

	hybridRetriever := retrieval.NewHybridRetriever(vectorRepo, embedder)
	ragService := service.NewRAGService(hybridRetriever, llm)

	ragService.SetProviderResolver(func(providerName string) (provider.LLMProvider, error) {
		name := strings.ToLower(strings.TrimSpace(providerName))
		switch name {
		case "ollama", "local":
			ollamaBaseURL := cfg.Providers.Defaults.OllamaBaseURL
			if ollamaBaseURL == "" {
				ollamaBaseURL = "http://localhost:11434"
			}
			return provider.NewOllamaLLMProvider(ollamaBaseURL, "qwen3.5:latest", 90*time.Second), nil
		case "openai":
			if cfg.Providers.APIKeys.OpenAI == "" {
				return nil, errors.New("OpenAI API key is not configured. Please set OPENAI_API_KEY in your .env file or select Ollama")
			}
			return provider.NewLLMProviderByName("openai", provider.FactoryConfig{
				APIKey:  cfg.Providers.APIKeys.OpenAI,
				Model:   "gpt-4o",
				Timeout: 60 * time.Second,
			})
		case "anthropic":
			if cfg.Providers.APIKeys.Anthropic == "" {
				return nil, errors.New("Anthropic API key is not configured. Please set ANTHROPIC_API_KEY in your .env file")
			}
			return provider.NewLLMProviderByName("anthropic", provider.FactoryConfig{
				APIKey:  cfg.Providers.APIKeys.Anthropic,
				Model:   "claude-3-5-sonnet",
				Timeout: 60 * time.Second,
			})
		case "gemini":
			if cfg.Providers.APIKeys.Gemini == "" {
				return nil, errors.New("Gemini API key is not configured. Please set GEMINI_API_KEY in your .env file")
			}
			return provider.NewLLMProviderByName("gemini", provider.FactoryConfig{
				APIKey:  cfg.Providers.APIKeys.Gemini,
				Model:   "gemini-1.5-pro",
				Timeout: 60 * time.Second,
			})
		case "opencode", "cli_opencode", "opencode-cli":
			binPath := cfg.Providers.CLIPaths.OpenCodeCLI
			if binPath == "" {
				binPath = "opencode"
			}
			if _, err := exec.LookPath(binPath); err != nil {
				return nil, fmt.Errorf("CLI binary %q not found in PATH. Install opencode or switch provider to Ollama", binPath)
			}
			return provider.NewLLMProviderByName("opencode", provider.FactoryConfig{
				BinaryPath: binPath,
				Timeout:    60 * time.Second,
			})
		case "mock":
			return provider.NewMockLLMProvider("ContextForge AI (Mock): ready to answer code questions."), nil
		default:
			return nil, fmt.Errorf("unsupported AI provider: %q", providerName)
		}
	})

	// 7. Initialize Handlers
	authH := handler.NewAuthHandler(authService, cfg.Auth.GithubPAT)
	projectH := handler.NewProjectHandler(projectRepo, log)
	sourceH := handler.NewSourceHandler(sourceRepo, jobRepo, queueClient, log)
	chunker := chunk.NewChunker(chunk.DefaultOptions())
	docH := handler.NewDocumentHandler(docRepo, vectorRepo, log)
	docUploadH := handler.NewDocumentUploadHandler(docRepo, sourceRepo, vectorRepo, embedder, chunker, log)
	jobH := handler.NewJobHandler(jobRepo, log)
	chatH := handler.NewChatHandler(ragService, log)
	chatH.SetConversationRepository(conversationRepo)
	conversationH := handler.NewConversationHandler(conversationRepo, log)
	webhookH := handler.NewWebhookHandler(cfg.Auth.WebhookSecret, log)

	encKey, err := crypto.KeyFromHex(cfg.Auth.TokenEncryptionKey)
	if err != nil {
		log.Fatal("invalid token encryption key", zap.Error(err))
	}
	allowPrivateIPs := cfg.Server.Env == "development"
	dbSourceService := service.NewDatabaseSourceService(
		db.EntClient,
		connector.DefaultRegistry(),
		encKey,
		queueClient,
		jobRepo,
		allowPrivateIPs,
		log,
	)
	dbSourceH := handler.NewDatabaseSourceHandler(dbSourceService, log)

	// 8. Initialize server router & dependency checkers
	server := api.NewServer(cfg, log, db, nil)
	server.MountRoutes(api.Handlers{
		Auth:           authH,
		Project:        projectH,
		Source:         sourceH,
		Document:       docH,
		DocumentUpload: docUploadH,
		Job:            jobH,
		Chat:           chatH,
		Conversation:   conversationH,
		Webhook:        webhookH,
		DatabaseSource: dbSourceH,
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
