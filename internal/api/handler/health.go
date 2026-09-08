package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// DependencyChecker defines readiness checks for stateful dependencies (DB, Redis).
type DependencyChecker interface {
	Check(ctx context.Context) error
}

// HealthHandler handles liveness and readiness probe requests.
type HealthHandler struct {
	dbChecker    DependencyChecker
	redisChecker DependencyChecker
}

// NewHealthHandler creates a HealthHandler with optional DB and Redis readiness checkers.
func NewHealthHandler(dbChecker, redisChecker DependencyChecker) *HealthHandler {
	return &HealthHandler{
		dbChecker:    dbChecker,
		redisChecker: redisChecker,
	}
}

// Healthz handles GET /healthz (Liveness probe).
func (h *HealthHandler) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// Readyz handles GET /readyz (Readiness probe).
func (h *HealthHandler) Readyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	status := gin.H{
		"status":   "ready",
		"database": "connected",
		"redis":    "connected",
	}

	var errors []string

	if h.dbChecker != nil {
		if err := h.dbChecker.Check(ctx); err != nil {
			status["database"] = "unhealthy"
			errors = append(errors, "database: "+err.Error())
		}
	}

	if h.redisChecker != nil {
		if err := h.redisChecker.Check(ctx); err != nil {
			status["redis"] = "unhealthy"
			errors = append(errors, "redis: "+err.Error())
		}
	}

	if len(errors) > 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"type":     "https://contextforge.dev/errors/dependency-unready",
			"title":    "Service Unavailable",
			"status":   http.StatusServiceUnavailable,
			"detail":   "One or more stateful dependencies are unready",
			"instance": c.Request.URL.Path,
			"errors":   errors,
			"state":    status,
		})
		return
	}

	c.JSON(http.StatusOK, status)
}
