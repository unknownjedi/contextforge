package api

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/api/handler"
	"github.com/your-org/contextforge/internal/api/middleware"
	"github.com/your-org/contextforge/internal/auth"
	"github.com/your-org/contextforge/internal/config"
	"github.com/your-org/contextforge/internal/logger"
	"github.com/your-org/contextforge/internal/repository"
)

// Handlers encapsulates all API handler instances for mounting onto the router.
type Handlers struct {
	Auth     *handler.AuthHandler
	Project  *handler.ProjectHandler
	Source   *handler.SourceHandler
	Document *handler.DocumentHandler
	Job            *handler.JobHandler
	Chat           *handler.ChatHandler
	Webhook        *handler.WebhookHandler
	DatabaseSource *handler.DatabaseSourceHandler
}

// Server encapsulates the HTTP router and server lifecycle.
type Server struct {
	router       *gin.Engine
	config       *config.Config
	logger       *zap.Logger
	rateLimiters []*middleware.RateLimiter
}

// NewServer configures Gin engine with middleware and routes.
func NewServer(cfg *config.Config, log *zap.Logger, dbChecker, redisChecker handler.DependencyChecker) *Server {
	if cfg.Server.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.Middleware(log))

	// CORS configuration
	corsCfg := cors.Config{
		AllowOrigins:     cfg.Server.CORS.AllowedOrigins,
		AllowMethods:     cfg.Server.CORS.AllowedMethods,
		AllowHeaders:     cfg.Server.CORS.AllowedHeaders,
		AllowCredentials: cfg.Server.CORS.AllowCredentials,
		MaxAge:           12 * time.Hour,
	}
	if len(corsCfg.AllowOrigins) == 0 {
		corsCfg.AllowAllOrigins = true
	}
	r.Use(cors.New(corsCfg))

	// Register System & Health routes
	healthH := handler.NewHealthHandler(dbChecker, redisChecker)
	r.GET("/healthz", healthH.Healthz)
	r.GET("/readyz", healthH.Readyz)

	return &Server{
		router: r,
		config: cfg,
		logger: log,
	}
}

// Router returns the underlying gin.Engine.
func (s *Server) Router() *gin.Engine {
	return s.router
}

// MountRoutes mounts all ContextForge API routes (/api/v1, /api/docs, /api/openapi.yaml) onto the engine.
func (s *Server) MountRoutes(h Handlers, projectRepo repository.ProjectRepository) {
	// 1. API Documentation routes
	s.router.GET("/api/openapi.yaml", func(c *gin.Context) {
		candidates := []string{
			"docs/api/openapi.yaml",
			"../docs/api/openapi.yaml",
			"../../docs/api/openapi.yaml",
			"/app/docs/api/openapi.yaml",
		}
		for _, path := range candidates {
			if data, err := os.ReadFile(path); err == nil {
				c.Data(http.StatusOK, "application/yaml; charset=utf-8", data)
				return
			}
		}
		c.String(http.StatusNotFound, "OpenAPI specification not found")
	})

	s.router.GET("/api/docs", func(c *gin.Context) {
		html := `<!doctype html>
<html>
  <head>
    <title>ContextForge API Documentation</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <link rel="icon" type="image/svg+xml" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>⚡</text></svg>">
  </head>
  <body>
    <script id="api-reference" data-url="/api/openapi.yaml"></script>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
  </body>
</html>`
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	})

	// 2. Rate limiters
	generalLimiter := middleware.NewRateLimiter(100.0/60.0, 100)
	chatLimiter := middleware.NewRateLimiter(20.0/60.0, 20)
	syncLimiter := middleware.NewRateLimiter(10.0/60.0, 10)
	s.rateLimiters = append(s.rateLimiters, generalLimiter, chatLimiter, syncLimiter)

	// 3. /api/v1 route group
	v1 := s.router.Group("/api/v1")
	v1.Use(generalLimiter.Middleware())

	// Public Auth & Webhooks
	if h.Auth != nil {
		v1.POST("/auth/pat", h.Auth.AuthenticatePAT)
		v1.GET("/auth/auto", h.Auth.AutoAuthenticateDev)
	}
	if h.Webhook != nil {
		v1.POST("/github/webhooks", h.Webhook.HandleWebhook)
		s.router.POST("/github/webhooks", h.Webhook.HandleWebhook)
	}

	// Authenticated endpoints
	jwtSecret := s.config.Auth.JWTSecret
	authed := v1.Group("")
	if jwtSecret != "" {
		authed.Use(auth.AuthMiddleware([]byte(jwtSecret)))
	}

	if h.Auth != nil {
		authed.GET("/auth/me", h.Auth.GetMe)
	}

	if h.Project != nil {
		authed.GET("/projects", h.Project.ListProjects)
		authed.POST("/projects", h.Project.CreateProject)

		projectGroup := authed.Group("/projects/:id")
		if projectRepo != nil {
			projectGroup.Use(middleware.RequireProjectAccess(projectRepo))
		}
		{
			projectGroup.GET("", h.Project.GetProject)
			projectGroup.PATCH("", h.Project.UpdateProject)
			projectGroup.DELETE("", h.Project.DeleteProject)

			if h.Source != nil {
				projectGroup.GET("/sources", h.Source.ListSources)
				projectGroup.POST("/sources", h.Source.CreateSource)
				projectGroup.DELETE("/sources/:source_id", h.Source.DeleteSource)
				projectGroup.POST("/sources/:source_id/sync", syncLimiter.Middleware(), h.Source.SyncSource)
			}

			if h.DatabaseSource != nil {
				dbGroup := projectGroup.Group("/sources/database")
				{
					dbGroup.POST("/test", h.DatabaseSource.TestRawConnection)
					dbGroup.POST("", h.DatabaseSource.CreateDatabaseSource)
					dbGroup.GET("", h.DatabaseSource.ListDatabaseSources)
					dbGroup.GET("/:source_id", h.DatabaseSource.GetDatabaseSource)
					dbGroup.PATCH("/:source_id", h.DatabaseSource.UpdateDatabaseSource)
					dbGroup.DELETE("/:source_id", h.DatabaseSource.DeleteDatabaseSource)
					dbGroup.POST("/:source_id/test", h.DatabaseSource.TestStoredConnection)
					dbGroup.GET("/:source_id/metadata", h.DatabaseSource.GetMetadata)
					dbGroup.POST("/:source_id/sync", syncLimiter.Middleware(), h.DatabaseSource.TriggerSync)
					dbGroup.GET("/:source_id/status", h.DatabaseSource.GetStatus)
				}
			}

			if h.Document != nil {
				projectGroup.GET("/documents", h.Document.ListDocuments)
				projectGroup.GET("/documents/:doc_id", h.Document.GetDocument)
				projectGroup.DELETE("/documents/:doc_id", h.Document.DeleteDocument)
			}

			if h.Job != nil {
				projectGroup.GET("/jobs/:job_id", h.Job.GetJob)
			}

			if h.Chat != nil {
				projectGroup.POST("/chat/completions/stream", chatLimiter.Middleware(), h.Chat.StreamChatCompletions)
				projectGroup.POST("/chat/completions", chatLimiter.Middleware(), h.Chat.ChatCompletions)
			}
		}
	}
}

// RegisterChatRoutes mounts chat completions streaming and sync endpoints.
func (s *Server) RegisterChatRoutes(chatH *handler.ChatHandler) {
	chatH.RegisterRoutes(s.router)
}

// HTTPServer constructs a production-configured standard *http.Server.
func (s *Server) HTTPServer() *http.Server {
	addr := fmt.Sprintf(":%d", s.config.Server.Port)
	return &http.Server{
		Addr:              addr,
		Handler:           s.router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      300 * time.Second, // Accommodate long-running SSE streams
		IdleTimeout:       120 * time.Second,
	}
}

// Close gracefully terminates background resources including rate limiters.
func (s *Server) Close() {
	for _, rl := range s.rateLimiters {
		if rl != nil {
			rl.Close()
		}
	}
}

