package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/repository"
)

// DocumentDetailResponse represents a document with its indexed vector chunks.
type DocumentDetailResponse struct {
	*ent.Document
	Chunks []*model.DocumentChunk `json:"chunks"`
}

// DocumentHandler handles document inspection endpoints.
type DocumentHandler struct {
	docRepo    repository.DocumentRepository
	vectorRepo repository.VectorRepository
	logger     *zap.Logger
}

// NewDocumentHandler constructs a DocumentHandler.
func NewDocumentHandler(
	docRepo repository.DocumentRepository,
	vectorRepo repository.VectorRepository,
	logger *zap.Logger,
) *DocumentHandler {
	return &DocumentHandler{
		docRepo:    docRepo,
		vectorRepo: vectorRepo,
		logger:     logger,
	}
}

// ListDocuments handles GET /projects/:id/documents
func (h *DocumentHandler) ListDocuments(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	search := c.Query("search")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	docs, total, err := h.docRepo.ListByProjectID(c.Request.Context(), projectID, search, page, pageSize)
	if err != nil {
		h.logger.Error("failed to list documents", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list documents"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     docs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetDocument handles GET /projects/:id/documents/:doc_id
func (h *DocumentHandler) GetDocument(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	docID, err := uuid.Parse(c.Param("doc_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid document ID"})
		return
	}

	doc, err := h.docRepo.GetByID(c.Request.Context(), docID, projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
		return
	}

	var chunks []*model.DocumentChunk
	if h.vectorRepo != nil {
		chunks, err = h.vectorRepo.GetChunksByDocumentID(c.Request.Context(), projectID, docID)
		if err != nil {
			h.logger.Warn("failed to fetch chunks from vector repo", zap.Error(err), zap.String("doc_id", docID.String()))
			chunks = []*model.DocumentChunk{}
		}
	}
	if chunks == nil {
		chunks = []*model.DocumentChunk{}
	}

	c.JSON(http.StatusOK, DocumentDetailResponse{
		Document: doc,
		Chunks:   chunks,
	})
}

// DeleteDocument handles DELETE /projects/:id/documents/:doc_id
func (h *DocumentHandler) DeleteDocument(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	docID, err := uuid.Parse(c.Param("doc_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid document ID"})
		return
	}

	if err := h.docRepo.Delete(c.Request.Context(), docID, projectID); err != nil {
		h.logger.Error("failed to delete document", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete document"})
		return
	}

	if h.vectorRepo != nil {
		_ = h.vectorRepo.DeleteChunksByDocumentID(c.Request.Context(), projectID, docID)
	}

	c.Status(http.StatusNoContent)
}
