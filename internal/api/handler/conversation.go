package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/repository"
)

// ConversationHandler handles project chat conversation management.
type ConversationHandler struct {
	repo   repository.ConversationRepository
	logger *zap.Logger
}

// NewConversationHandler constructs a new ConversationHandler.
func NewConversationHandler(repo repository.ConversationRepository, logger *zap.Logger) *ConversationHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ConversationHandler{
		repo:   repo,
		logger: logger,
	}
}

// CreateConversationRequest defines the optional request payload when creating a conversation.
type CreateConversationRequest struct {
	Title string `json:"title"`
}

// ConversationDetailResponse represents a conversation with its messages.
type ConversationDetailResponse struct {
	ID        uuid.UUID          `json:"id"`
	ProjectID uuid.UUID          `json:"project_id"`
	Title     string             `json:"title"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Messages  []*ent.ChatMessage `json:"messages"`
}

// ListConversations handles GET /projects/:id/conversations.
func (h *ConversationHandler) ListConversations(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID format"})
		return
	}

	convs, err := h.repo.ListByProjectID(c.Request.Context(), projectID)
	if err != nil {
		h.logger.Error("failed to list conversations", zap.Error(err), zap.String("project_id", projectID.String()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list conversations"})
		return
	}
	if convs == nil {
		convs = []*ent.Conversation{}
	}

	c.JSON(http.StatusOK, convs)
}

// CreateConversation handles POST /projects/:id/conversations.
func (h *ConversationHandler) CreateConversation(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID format"})
		return
	}

	var req CreateConversationRequest
	// Body is optional; ignoring EOF/empty body errors
	_ = c.ShouldBindJSON(&req)

	conv, err := h.repo.CreateConversation(c.Request.Context(), projectID, req.Title)
	if err != nil {
		h.logger.Error("failed to create conversation", zap.Error(err), zap.String("project_id", projectID.String()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create conversation"})
		return
	}

	c.JSON(http.StatusCreated, conv)
}

// GetConversation handles GET /projects/:id/conversations/:conv_id.
func (h *ConversationHandler) GetConversation(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID format"})
		return
	}

	convID, err := uuid.Parse(c.Param("conv_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation ID format"})
		return
	}

	conv, err := h.repo.GetByID(c.Request.Context(), convID, projectID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}

	messages := conv.Edges.Messages
	if messages == nil {
		// Fall back to listing messages if not eager loaded
		var listErr error
		messages, listErr = h.repo.ListMessages(c.Request.Context(), convID)
		if listErr != nil {
			h.logger.Warn("failed to list messages for conversation", zap.Error(listErr), zap.String("conv_id", convID.String()))
			messages = []*ent.ChatMessage{}
		}
	}
	if messages == nil {
		messages = []*ent.ChatMessage{}
	}

	c.JSON(http.StatusOK, ConversationDetailResponse{
		ID:        conv.ID,
		ProjectID: conv.ProjectID,
		Title:     conv.Title,
		CreatedAt: conv.CreatedAt,
		UpdatedAt: conv.UpdatedAt,
		Messages:  messages,
	})
}

// DeleteConversation handles DELETE /projects/:id/conversations/:conv_id.
func (h *ConversationHandler) DeleteConversation(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID format"})
		return
	}

	convID, err := uuid.Parse(c.Param("conv_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation ID format"})
		return
	}

	if err := h.repo.Delete(c.Request.Context(), convID, projectID); err != nil {
		h.logger.Warn("failed to delete conversation", zap.Error(err), zap.String("conv_id", convID.String()))
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found or unauthorized"})
		return
	}

	c.Status(http.StatusNoContent)
}
