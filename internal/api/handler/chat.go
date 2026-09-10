package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/repository"
)

// RAGService defines the domain interface consumed by ChatHandler.
type RAGService interface {
	Generate(ctx context.Context, projectID uuid.UUID, req *model.ChatRequest) (*model.ChatResponse, error)
	StreamChat(
		ctx context.Context,
		projectID uuid.UUID,
		req *model.ChatRequest,
		onCitation func(*model.Citation) error,
		onToken func(string) error,
	) (*model.ChatResponse, error)
}

// ChatHandler exposes RAG chat completions endpoints for both SSE streaming and synchronous JSON.
type ChatHandler struct {
	ragService       RAGService
	conversationRepo repository.ConversationRepository
	logger           *zap.Logger
}

// NewChatHandler constructs a new ChatHandler.
func NewChatHandler(ragService RAGService, logger *zap.Logger) *ChatHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &ChatHandler{
		ragService: ragService,
		logger:     logger,
	}
}

// SetConversationRepository sets the repository used to persist conversation messages.
func (h *ChatHandler) SetConversationRepository(repo repository.ConversationRepository) {
	h.conversationRepo = repo
}

// RegisterRoutes registers the chat completion endpoints on the given gin router or router group.
func (h *ChatHandler) RegisterRoutes(r gin.IRoutes) {
	r.POST("/projects/:id/chat/completions/stream", h.StreamChatCompletions)
	r.POST("/projects/:id/chat/completions", h.ChatCompletions)
	r.POST("/api/v1/projects/:id/chat/completions/stream", h.StreamChatCompletions)
	r.POST("/api/v1/projects/:id/chat/completions", h.ChatCompletions)
}

// StreamChatCompletions handles POST /projects/:id/chat/completions/stream.
// It establishes a Server-Sent Events (SSE) stream, delivering:
//   - event: citation (per retrieved context reference)
//   - event: message  (incremental token delta)
//   - event: done     (total tokens and duration)
func (h *ChatHandler) StreamChatCompletions(c *gin.Context) {
	projectIDStr := c.Param("id")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID format"})
		return
	}

	var req model.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request payload: %s", err.Error())})
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message cannot be empty"})
		return
	}

	var convID uuid.UUID
	if req.ConversationID != nil && *req.ConversationID != uuid.Nil {
		convID = *req.ConversationID
	}

	if convID != uuid.Nil && h.conversationRepo != nil {
		conv, err := h.conversationRepo.GetByID(c.Request.Context(), convID, projectID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}

		if _, err := h.conversationRepo.CreateMessage(c.Request.Context(), convID, "user", req.Message, nil, 0, 0); err != nil {
			h.logger.Error("failed to save user message", zap.Error(err), zap.String("conv_id", convID.String()))
		}

		if conv.Title == "New Chat" {
			runes := []rune(strings.TrimSpace(req.Message))
			newTitle := string(runes)
			if len(runes) > 40 {
				newTitle = string(runes[:40])
			}
			if newTitle != "" {
				_, _ = h.conversationRepo.UpdateTitle(c.Request.Context(), convID, projectID, newTitle)
			}
		}
	}

	// Configure headers for text/event-stream
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		h.logger.Error("client writer does not support flushing")
		return
	}
	flusher.Flush()

	onCitation := func(cit *model.Citation) error {
		data, err := json.Marshal(cit)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "event: citation\ndata: %s\n\n", data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	onToken := func(delta string) error {
		data, err := json.Marshal(model.ChatStreamMessageEvent{Delta: delta})
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(c.Writer, "event: message\ndata: %s\n\n", data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	resp, err := h.ragService.StreamChat(c.Request.Context(), projectID, &req, onCitation, onToken)
	if err != nil {
		h.logger.Error("failed during stream chat completion",
			zap.Error(err),
			zap.String("project_id", projectID.String()),
		)
		errData, _ := json.Marshal(gin.H{"error": err.Error()})
		_, _ = fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", errData)
		flusher.Flush()
		return
	}

	if convID != uuid.Nil && h.conversationRepo != nil && resp != nil {
		var citations []model.Citation
		if resp.Citations != nil {
			citations = resp.Citations
		}
		if _, err := h.conversationRepo.CreateMessage(
			c.Request.Context(),
			convID,
			"assistant",
			resp.Answer,
			citations,
			resp.TokensUsed,
			resp.DurationMs,
		); err != nil {
			h.logger.Error("failed to save assistant message", zap.Error(err), zap.String("conv_id", convID.String()))
		}
	}

	var tokensUsed int
	var durationMs int64
	if resp != nil {
		tokensUsed = resp.TokensUsed
		durationMs = resp.DurationMs
	}
	doneData, err := json.Marshal(model.ChatStreamDoneEvent{
		TotalTokens: tokensUsed,
		DurationMs:  durationMs,
	})
	if err == nil {
		_, _ = fmt.Fprintf(c.Writer, "event: done\ndata: %s\n\n", doneData)
		flusher.Flush()
	}
}

// ChatCompletions handles POST /projects/:id/chat/completions (synchronous JSON query).
func (h *ChatHandler) ChatCompletions(c *gin.Context) {
	projectIDStr := c.Param("id")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID format"})
		return
	}

	var req model.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid request payload: %s", err.Error())})
		return
	}

	if strings.TrimSpace(req.Message) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message cannot be empty"})
		return
	}

	var convID uuid.UUID
	if req.ConversationID != nil && *req.ConversationID != uuid.Nil {
		convID = *req.ConversationID
	}

	if convID != uuid.Nil && h.conversationRepo != nil {
		conv, err := h.conversationRepo.GetByID(c.Request.Context(), convID, projectID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}

		if _, err := h.conversationRepo.CreateMessage(c.Request.Context(), convID, "user", req.Message, nil, 0, 0); err != nil {
			h.logger.Error("failed to save user message", zap.Error(err), zap.String("conv_id", convID.String()))
		}

		if conv.Title == "New Chat" {
			runes := []rune(strings.TrimSpace(req.Message))
			newTitle := string(runes)
			if len(runes) > 40 {
				newTitle = string(runes[:40])
			}
			if newTitle != "" {
				_, _ = h.conversationRepo.UpdateTitle(c.Request.Context(), convID, projectID, newTitle)
			}
		}
	}

	resp, err := h.ragService.Generate(c.Request.Context(), projectID, &req)
	if err != nil {
		h.logger.Error("chat completion failed",
			zap.Error(err),
			zap.String("project_id", projectID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if convID != uuid.Nil && h.conversationRepo != nil && resp != nil {
		var citations []model.Citation
		if resp.Citations != nil {
			citations = resp.Citations
		}
		if _, err := h.conversationRepo.CreateMessage(
			c.Request.Context(),
			convID,
			"assistant",
			resp.Answer,
			citations,
			resp.TokensUsed,
			resp.DurationMs,
		); err != nil {
			h.logger.Error("failed to save assistant message", zap.Error(err), zap.String("conv_id", convID.String()))
		}
	}

	c.JSON(http.StatusOK, resp)
}
