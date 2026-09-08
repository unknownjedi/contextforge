package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/auth"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/repository"
)

const (
	ContextKeyProject   = "tenant_project"
	ContextKeyProjectID = "tenant_project_id"
)

var (
	ErrProjectNotFound   = errors.New("project not found or unauthorized")
	ErrInvalidProjectID  = errors.New("invalid project ID")
	ErrProjectNotInCtx   = errors.New("project not found in context")
)

// RequireProjectAccess verifies that the authenticated user owns the requested project.
// In v1, single-user ownership is strictly enforced: owner_user_id == authenticated_user_id.
// Non-existent projects or projects owned by another tenant return 404 to prevent resource enumeration.
func RequireProjectAccess(projectRepo repository.ProjectRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Resolve Project ID from URL parameter
		paramID := c.Param("id")
		if paramID == "" {
			paramID = c.Param("project_id")
		}

		projectID, err := uuid.Parse(paramID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"type":     "https://contextforge.dev/errors/validation",
				"title":    "Invalid Project ID",
				"status":   http.StatusBadRequest,
				"detail":   "The provided project ID is not a valid UUID",
				"instance": c.Request.URL.Path,
			})
			return
		}

		// 2. Resolve Authenticated User ID from context
		userID, err := auth.GetUserID(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"type":     "https://contextforge.dev/errors/unauthorized",
				"title":    "Unauthorized",
				"status":   http.StatusUnauthorized,
				"detail":   "User authentication required to access project",
				"instance": c.Request.URL.Path,
			})
			return
		}

		// 3. Query project with strict owner_user_id filter
		p, err := projectRepo.GetByID(c.Request.Context(), projectID, userID)
		if err != nil || p == nil {
			// Return 404 to obscure existence of other tenants' projects
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
				"type":     "https://contextforge.dev/errors/not-found",
				"title":    "Project Not Found",
				"status":   http.StatusNotFound,
				"detail":   "The requested project does not exist or you do not have permission to access it",
				"instance": c.Request.URL.Path,
			})
			return
		}

		// 4. Inject project into context
		c.Set(ContextKeyProject, p)
		c.Set(ContextKeyProjectID, p.ID)
		c.Next()
	}
}

// GetProject extracts the verified *ent.Project from the Gin context.
func GetProject(c *gin.Context) (*ent.Project, error) {
	val, exists := c.Get(ContextKeyProject)
	if !exists {
		return nil, ErrProjectNotInCtx
	}
	p, ok := val.(*ent.Project)
	if !ok || p == nil {
		return nil, ErrProjectNotInCtx
	}
	return p, nil
}

// GetProjectID extracts the verified project UUID from the Gin context.
func GetProjectID(c *gin.Context) (uuid.UUID, error) {
	val, exists := c.Get(ContextKeyProjectID)
	if !exists {
		return uuid.Nil, ErrProjectNotInCtx
	}
	id, ok := val.(uuid.UUID)
	if !ok || id == uuid.Nil {
		return uuid.Nil, ErrProjectNotInCtx
	}
	return id, nil
}
