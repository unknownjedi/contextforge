package handler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ent/ingestionjob"
	"github.com/your-org/contextforge/internal/ent/source"
	"github.com/your-org/contextforge/internal/queue"
	"github.com/your-org/contextforge/internal/repository"
	"github.com/your-org/contextforge/internal/urlfetch"
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

// CreateSourceRequest defines payload for adding a new code or URL source.
type CreateSourceRequest struct {
	Name      string `json:"name" binding:"required"`
	Type      string `json:"type"` // "github" (default) or "url"
	URL       string `json:"url"`
	RepoOwner string `json:"repo_owner"`
	RepoName  string `json:"repo_name"`
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

	sourceType := strings.ToLower(strings.TrimSpace(req.Type))
	if sourceType == "" {
		sourceType = "github"
	}

	if sourceType == "url" {
		trimmedURL := strings.TrimSpace(req.URL)
		if trimmedURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "url is required for url sources"})
			return
		}

		// Validate SSRF
		if _, err := urlfetch.ValidateURL(trimmedURL); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid URL: %v", err)})
			return
		}

		s := &ent.Source{
			ID:         uuid.New(),
			ProjectID:  projectID,
			Name:       req.Name,
			Type:       "url",
			RepoOwner:  "",
			RepoName:   trimmedURL,
			Branch:     "",
			SyncStatus: source.SyncStatusQueued,
		}

		created, err := h.sourceRepo.Create(c.Request.Context(), s)
		if err != nil {
			h.logger.Error("failed to create url source", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create source"})
			return
		}

		jobID := uuid.New()
		if h.jobRepo != nil {
			job := &ent.IngestionJob{
				ID:              jobID,
				ProjectID:       projectID,
				SourceID:        created.ID,
				Status:          ingestionjob.StatusPending,
				ProcessedFiles:  0,
				TotalFiles:      1,
				ProgressPercent: 0,
			}
			if _, err := h.jobRepo.Create(c.Request.Context(), job); err != nil {
				h.logger.Error("failed to create initial ingestion job record", zap.Error(err))
			}
		}

		if h.queueClient != nil {
			_, err := h.queueClient.EnqueueURLSync(c.Request.Context(), queue.URLSyncPayload{
				JobID:     jobID,
				ProjectID: projectID,
				SourceID:  created.ID,
				URL:       trimmedURL,
			})
			if err != nil {
				h.logger.Error("failed to enqueue url sync task", zap.Error(err))
			}
		}

		c.JSON(http.StatusCreated, gin.H{
			"id":               created.ID,
			"project_id":       created.ProjectID,
			"name":             created.Name,
			"type":             created.Type,
			"repo_owner":       created.RepoOwner,
			"repo_name":        created.RepoName,
			"branch":           created.Branch,
			"sync_status":      created.SyncStatus,
			"last_commit_hash": created.LastCommitHash,
			"created_at":       created.CreatedAt,
			"updated_at":       created.UpdatedAt,
			"source":           created,
			"job_id":           jobID,
		})
		return
	}

	// For github sources:
	if req.RepoOwner == "" || req.RepoName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "repo_owner and repo_name are required for github sources"})
		return
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

	// Create initial ingestion job record
	jobID := uuid.New()
	if h.jobRepo != nil {
		job := &ent.IngestionJob{
			ID:              jobID,
			ProjectID:       projectID,
			SourceID:        created.ID,
			Status:          ingestionjob.StatusPending,
			ProcessedFiles:  0,
			TotalFiles:      0,
			ProgressPercent: 0,
		}
		if _, err := h.jobRepo.Create(c.Request.Context(), job); err != nil {
			h.logger.Error("failed to create initial ingestion job record", zap.Error(err))
		}
	}

	// Enqueue sync task in worker queue
	if h.queueClient != nil {
		_, err := h.queueClient.EnqueueRepoSync(c.Request.Context(), queue.RepoSyncPayload{
			JobID:     jobID,
			ProjectID: projectID,
			SourceID:  created.ID,
		})
		if err != nil {
			h.logger.Error("failed to enqueue initial sync task", zap.Error(err))
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":               created.ID,
		"project_id":       created.ProjectID,
		"name":             created.Name,
		"type":             created.Type,
		"repo_owner":       created.RepoOwner,
		"repo_name":        created.RepoName,
		"branch":           created.Branch,
		"sync_status":      created.SyncStatus,
		"last_commit_hash": created.LastCommitHash,
		"created_at":       created.CreatedAt,
		"updated_at":       created.UpdatedAt,
		"source":           created,
		"job_id":           jobID,
	})
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

	if src.Type != "" && src.Type != "github" && src.Type != "url" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot trigger repository sync for non-github and non-url source; use database sync endpoint"})
		return
	}

	totalFiles := 0
	if src.Type == "url" {
		totalFiles = 1
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
			TotalFiles:      totalFiles,
			ProgressPercent: 0,
		}
		if _, err := h.jobRepo.Create(c.Request.Context(), job); err != nil {
			h.logger.Error("failed to create ingestion job record", zap.Error(err))
		}
	}

	// Enqueue sync task
	if h.queueClient != nil {
		if src.Type == "url" {
			_, err := h.queueClient.EnqueueURLSync(c.Request.Context(), queue.URLSyncPayload{
				JobID:     jobID,
				ProjectID: projectID,
				SourceID:  sourceID,
				URL:       src.RepoName,
			})
			if err != nil {
				h.logger.Error("failed to enqueue url sync task", zap.Error(err))
			}
		} else {
			_, err := h.queueClient.EnqueueRepoSync(c.Request.Context(), queue.RepoSyncPayload{
				JobID:     jobID,
				ProjectID: projectID,
				SourceID:  sourceID,
			})
			if err != nil {
				h.logger.Error("failed to enqueue sync task", zap.Error(err))
			}
		}
	}

	// Update source status to syncing
	_, _ = h.sourceRepo.UpdateSyncStatus(c.Request.Context(), sourceID, projectID, source.SyncStatusSyncing, src.LastCommitHash, nil)

	now := time.Now()
	c.JSON(http.StatusAccepted, gin.H{
		"id":               jobID,
		"job_id":           jobID,
		"project_id":       projectID,
		"source_id":        sourceID,
		"status":           "queued",
		"processed_files":  0,
		"total_files":      totalFiles,
		"progress_percent": 0,
		"created_at":       now,
		"updated_at":       now,
	})
}
