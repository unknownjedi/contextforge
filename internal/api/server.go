package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/api/handler"
	"github.com/your-org/contextforge/internal/config"
	"github.com/your-org/contextforge/internal/logger"
)

// Server encapsulates the HTTP router and server lifecycle.
type Server struct {
	router *gin.Engine
	config *config.Config
	logger *zap.Logger
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
