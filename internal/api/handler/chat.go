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
	ragService RAGService
	logger     *zap.Logger
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

	doneData, err := json.Marshal(model.ChatStreamDoneEvent{
		TotalTokens: resp.TokensUsed,
		DurationMs:  resp.DurationMs,
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

	resp, err := h.ragService.Generate(c.Request.Context(), projectID, &req)
	if err != nil {
		h.logger.Error("chat completion failed",
			zap.Error(err),
			zap.String("project_id", projectID.String()),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}
