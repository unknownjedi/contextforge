package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/service"
)

// DatabaseSourceHandler handles API requests for external database knowledge sources.
type DatabaseSourceHandler struct {
	dbSourceSvc *service.DatabaseSourceService
	logger      *zap.Logger
}

// NewDatabaseSourceHandler constructs a DatabaseSourceHandler.
func NewDatabaseSourceHandler(dbSourceSvc *service.DatabaseSourceService, logger *zap.Logger) *DatabaseSourceHandler {
	return &DatabaseSourceHandler{
		dbSourceSvc: dbSourceSvc,
		logger:      logger,
	}
}

// TestRawConnectionRequest holds payload for ad-hoc connection test.
type TestRawConnectionRequest struct {
	DatabaseType  string `json:"database_type" binding:"required"`
	ConnectionURL string `json:"connection_url" binding:"required"`
}

// TestRawConnection handles POST /projects/:id/sources/database/test
func (h *DatabaseSourceHandler) TestRawConnection(c *gin.Context) {
	if _, err := uuid.Parse(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	var req TestRawConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.dbSourceSvc.TestConnection(c.Request.Context(), req.DatabaseType, req.ConnectionURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success":       false,
			"database_type": req.DatabaseType,
			"error_message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// CreateDatabaseSource handles POST /projects/:id/sources/database
func (h *DatabaseSourceHandler) CreateDatabaseSource(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	var req service.CreateDatabaseSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.dbSourceSvc.CreateDatabaseSource(c.Request.Context(), projectID, req)
	if err != nil {
		h.logger.Error("failed to create database source", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, created)
}

// ListDatabaseSources handles GET /projects/:id/sources/database
func (h *DatabaseSourceHandler) ListDatabaseSources(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	sources, err := h.dbSourceSvc.ListDatabaseSources(c.Request.Context(), projectID)
	if err != nil {
		h.logger.Error("failed to list database sources", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list database sources"})
		return
	}

	c.JSON(http.StatusOK, sources)
}

// GetDatabaseSource handles GET /projects/:id/sources/database/:source_id
func (h *DatabaseSourceHandler) GetDatabaseSource(c *gin.Context) {
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

	src, err := h.dbSourceSvc.GetDatabaseSource(c.Request.Context(), projectID, sourceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "database source not found"})
		return
	}

	c.JSON(http.StatusOK, src)
}

// UpdateDatabaseSource handles PATCH /projects/:id/sources/database/:source_id
func (h *DatabaseSourceHandler) UpdateDatabaseSource(c *gin.Context) {
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

	var req service.UpdateDatabaseSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.dbSourceSvc.UpdateDatabaseSource(c.Request.Context(), projectID, sourceID, req)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database source not found"})
			return
		}
		h.logger.Error("failed to update database source", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update database source"})
		return
	}

	c.JSON(http.StatusOK, updated)
}

// DeleteDatabaseSource handles DELETE /projects/:id/sources/database/:source_id
func (h *DatabaseSourceHandler) DeleteDatabaseSource(c *gin.Context) {
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

	if err := h.dbSourceSvc.DeleteDatabaseSource(c.Request.Context(), projectID, sourceID); err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database source not found"})
			return
		}
		h.logger.Error("failed to delete database source", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete database source"})
		return
	}

	c.Status(http.StatusNoContent)
}

// TestStoredConnection handles POST /projects/:id/sources/database/:source_id/test
func (h *DatabaseSourceHandler) TestStoredConnection(c *gin.Context) {
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

	result, err := h.dbSourceSvc.TestStoredConnection(c.Request.Context(), projectID, sourceID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database source not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success":       false,
			"error_message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetMetadata handles GET /projects/:id/sources/database/:source_id/metadata
func (h *DatabaseSourceHandler) GetMetadata(c *gin.Context) {
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

	metadata, err := h.dbSourceSvc.GetMetadata(c.Request.Context(), projectID, sourceID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database source not found"})
			return
		}
		h.logger.Error("failed to retrieve database metadata", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, metadata)
}

// TriggerSync handles POST /projects/:id/sources/database/:source_id/sync
func (h *DatabaseSourceHandler) TriggerSync(c *gin.Context) {
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

	jobID, err := h.dbSourceSvc.TriggerSync(c.Request.Context(), projectID, sourceID)
	if err != nil {
		if ent.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "database source not found"})
			return
		}
		h.logger.Error("failed to trigger database sync", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to trigger sync"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"job_id": jobID,
		"status": "pending",
	})
}

// GetStatus handles GET /projects/:id/sources/database/:source_id/status
func (h *DatabaseSourceHandler) GetStatus(c *gin.Context) {
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

	src, err := h.dbSourceSvc.GetDatabaseSource(c.Request.Context(), projectID, sourceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "database source not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"source_id":      src.SourceID,
		"status":         src.Status,
		"last_error":     src.LastError,
		"last_synced_at": src.LastSyncedAt,
	})
}
