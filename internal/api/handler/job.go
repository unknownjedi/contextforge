package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/repository"
)

// JobHandler handles ingestion job polling and monitoring.
type JobHandler struct {
	jobRepo repository.JobRepository
	logger  *zap.Logger
}

// NewJobHandler constructs a JobHandler.
func NewJobHandler(jobRepo repository.JobRepository, logger *zap.Logger) *JobHandler {
	return &JobHandler{
		jobRepo: jobRepo,
		logger:  logger,
	}
}

// GetJob handles GET /projects/:id/jobs/:job_id
func (h *JobHandler) GetJob(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	jobID, err := uuid.Parse(c.Param("job_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job ID"})
		return
	}

	job, err := h.jobRepo.GetByID(c.Request.Context(), jobID, projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	c.JSON(http.StatusOK, job)
}
