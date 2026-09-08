package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/ingestionjob"
	"github.com/your-org/contextforge/internal/ent/source"
	"github.com/your-org/contextforge/internal/queue"
	"github.com/your-org/contextforge/internal/repository"
)

// SourceHandler handles repository sources and synchronization triggers.
type SourceHandler struct {
	sourceRepo  repository.SourceRepository
	jobRepo     repository.JobRepository
	queueClient *queue.Client
	logger      *zap.Logger
}

// NewSourceHandler constructs a SourceHandler.
func NewSourceHandler(
	sourceRepo repository.SourceRepository,
	jobRepo repository.JobRepository,
	queueClient *queue.Client,
	logger *zap.Logger,
) *SourceHandler {
	return &SourceHandler{
		sourceRepo:  sourceRepo,
		jobRepo:     jobRepo,
		queueClient: queueClient,
		logger:      logger,
	}
}

// CreateSourceRequest defines payload for adding a new code source.
type CreateSourceRequest struct {
	Name      string `json:"name" binding:"required"`
	Type      string `json:"type"`      // "github" (default); omit or leave empty to use default
	RepoOwner string `json:"repo_owner" binding:"required"`
	RepoName  string `json:"repo_name" binding:"required"`
	Branch    string `json:"branch"`
}

// ListSources handles GET /projects/:id/sources
func (h *SourceHandler) ListSources(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	sources, err := h.sourceRepo.ListByProjectID(c.Request.Context(), projectID)
	if err != nil {
		h.logger.Error("failed to list sources", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sources"})
		return
	}

	c.JSON(http.StatusOK, sources)
}

// CreateSource handles POST /projects/:id/sources
func (h *SourceHandler) CreateSource(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	var req CreateSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	sourceType := req.Type
	if sourceType == "" {
		sourceType = "github"
	}

	branch := req.Branch
	if branch == "" {
		branch = "main"
	}

	s := &ent.Source{
		ID:         uuid.New(),
		ProjectID:  projectID,
		Name:       req.Name,
		Type:       sourceType,
		RepoOwner:  req.RepoOwner,
		RepoName:   req.RepoName,
		Branch:     branch,
		SyncStatus: source.SyncStatusQueued,
	}

	created, err := h.sourceRepo.Create(c.Request.Context(), s)
	if err != nil {
		h.logger.Error("failed to create source", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create source"})
		return
	}

	c.JSON(http.StatusCreated, created)
}

// DeleteSource handles DELETE /projects/:id/sources/:source_id
func (h *SourceHandler) DeleteSource(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	sourceID, err := uuid.Parse(c.Param("source_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source ID"})
		return
	}

	if err := h.sourceRepo.Delete(c.Request.Context(), sourceID, projectID); err != nil {
		h.logger.Error("failed to delete source", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete source"})
		return
	}

	c.Status(http.StatusNoContent)
}

// SyncSource handles POST /projects/:id/sources/:source_id/sync
func (h *SourceHandler) SyncSource(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	sourceID, err := uuid.Parse(c.Param("source_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source ID"})
		return
	}

	// Verify source exists
	src, err := h.sourceRepo.GetByID(c.Request.Context(), sourceID, projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "source not found"})
		return
	}

	if src.Type != "" && src.Type != "github" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot trigger repository sync for non-github source; use database sync endpoint"})
		return
	}

	// Create ingestion job
	jobID := uuid.New()
	if h.jobRepo != nil {
		job := &ent.IngestionJob{
			ID:              jobID,
			ProjectID:       projectID,
			SourceID:        sourceID,
			Status:          ingestionjob.StatusPending,
			ProcessedFiles:  0,
			TotalFiles:      0,
			ProgressPercent: 0,
		}
		if _, err := h.jobRepo.Create(c.Request.Context(), job); err != nil {
			h.logger.Error("failed to create ingestion job record", zap.Error(err))
		}
	}

	// Enqueue sync task
	if h.queueClient != nil {
		_, err := h.queueClient.EnqueueRepoSync(c.Request.Context(), queue.RepoSyncPayload{
			JobID:     jobID,
			ProjectID: projectID,
			SourceID:  sourceID,
		})
		if err != nil {
			h.logger.Error("failed to enqueue sync task", zap.Error(err))
		}
	}

	// Update source status to syncing
	_, _ = h.sourceRepo.UpdateSyncStatus(c.Request.Context(), sourceID, projectID, source.SyncStatusSyncing, src.LastCommitHash, nil)

	c.JSON(http.StatusAccepted, gin.H{
		"job_id":    jobID,
		"source_id": sourceID,
		"status":    "queued",
	})
}
