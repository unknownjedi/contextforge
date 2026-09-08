package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/auth"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/repository"
)

// ProjectHandler handles project CRUD endpoints.
type ProjectHandler struct {
	repo   repository.ProjectRepository
	logger *zap.Logger
}

// NewProjectHandler constructs a ProjectHandler.
func NewProjectHandler(repo repository.ProjectRepository, logger *zap.Logger) *ProjectHandler {
	return &ProjectHandler{
		repo:   repo,
		logger: logger,
	}
}

// CreateProjectRequest defines payload for project creation.
type CreateProjectRequest struct {
	Name               string `json:"name" binding:"required"`
	Description        string `json:"description"`
	EmbeddingProvider  string `json:"embedding_provider"`
	EmbeddingModel     string `json:"embedding_model"`
	EmbeddingDimension int    `json:"embedding_dimension"`
	LLMProvider        string `json:"llm_provider"`
	LLMModel           string `json:"llm_model"`
}

// UpdateProjectRequest defines payload for project update.
type UpdateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	LLMProvider *string `json:"llm_provider"`
	LLMModel    *string `json:"llm_model"`
}

// ListProjects handles GET /projects
func (h *ProjectHandler) ListProjects(c *gin.Context) {
	userID, err := auth.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	projects, total, err := h.repo.ListByOwner(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		h.logger.Error("failed to list projects", zap.Error(err), zap.String("user_id", userID.String()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list projects"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     projects,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// CreateProject handles POST /projects
func (h *ProjectHandler) CreateProject(c *gin.Context) {
	userID, err := auth.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.EmbeddingProvider == "" {
		req.EmbeddingProvider = "ollama"
	}
	if req.EmbeddingModel == "" {
		req.EmbeddingModel = "nomic-embed-text"
	}
	if req.EmbeddingDimension <= 0 {
		req.EmbeddingDimension = 768
	}
	if req.LLMProvider == "" {
		req.LLMProvider = "openai"
	}
	if req.LLMModel == "" {
		req.LLMModel = "gpt-4o"
	}

	p := &ent.Project{
		ID:                 uuid.New(),
		Name:               req.Name,
		Description:        req.Description,
		OwnerUserID:        userID,
		EmbeddingProvider:  req.EmbeddingProvider,
		EmbeddingModel:     req.EmbeddingModel,
		EmbeddingDimension: req.EmbeddingDimension,
		LlmProvider:        req.LLMProvider,
		LlmModel:           req.LLMModel,
	}

	created, err := h.repo.Create(c.Request.Context(), p)
	if err != nil {
		h.logger.Error("failed to create project", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create project"})
		return
	}

	c.JSON(http.StatusCreated, created)
}

// GetProject handles GET /projects/:id
func (h *ProjectHandler) GetProject(c *gin.Context) {
	userID, err := auth.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	p, err := h.repo.GetByID(c.Request.Context(), projectID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	c.JSON(http.StatusOK, p)
}

// UpdateProject handles PATCH /projects/:id
func (h *ProjectHandler) UpdateProject(c *gin.Context) {
	userID, err := auth.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	existing, err := h.repo.GetByID(c.Request.Context(), projectID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}

	var req UpdateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.LLMProvider != nil {
		existing.LlmProvider = *req.LLMProvider
	}
	if req.LLMModel != nil {
		existing.LlmModel = *req.LLMModel
	}

	updated, err := h.repo.Update(c.Request.Context(), existing, userID)
	if err != nil {
		h.logger.Error("failed to update project", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update project"})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteProject handles DELETE /projects/:id
func (h *ProjectHandler) DeleteProject(c *gin.Context) {
	userID, err := auth.GetUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	if err := h.repo.Delete(c.Request.Context(), projectID, userID); err != nil {
		h.logger.Error("failed to delete project", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete project"})
		return
	}

	c.Status(http.StatusNoContent)
}
