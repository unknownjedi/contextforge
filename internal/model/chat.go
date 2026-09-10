package model

import "github.com/google/uuid"

// Citation represents a structured reference anchored to source code or document lines.
// Matches openapi.yaml #/components/schemas/Citation
type Citation struct {
	SourceID   uuid.UUID `json:"source_id"`
	FilePath   string    `json:"file_path"`
	StartLine  int       `json:"start_line"`
	EndLine    int       `json:"end_line"`
	Similarity float32   `json:"similarity"`
	Snippet    string    `json:"snippet"`
}

// ChatRequest represents the payload for RAG chat queries.
// Matches openapi.yaml #/components/schemas/ChatCompletionRequest
type ChatRequest struct {
	Message             string     `json:"message" binding:"required"`
	ConversationID      *uuid.UUID `json:"conversation_id,omitempty"`
	TopK                int        `json:"top_k,omitempty"`
	SimilarityThreshold float32    `json:"similarity_threshold,omitempty"`
	FileFilters         []string   `json:"file_filters,omitempty"`
	Temperature         float32    `json:"temperature,omitempty"`
	Model               string     `json:"model,omitempty"`
	Provider            string     `json:"provider,omitempty"`
}

// ChatCompletionRequest is an alias for ChatRequest to align with OpenAPI naming.
type ChatCompletionRequest = ChatRequest

// ChatResponse represents the result of a synchronous RAG query.
// Matches openapi.yaml #/components/schemas/ChatCompletionResponse
type ChatResponse struct {
	Answer     string     `json:"answer"`
	Citations  []Citation `json:"citations"`
	TokensUsed int        `json:"tokens_used"`
	DurationMs int64      `json:"duration_ms"`
}

// ChatCompletionResponse is an alias for ChatResponse to align with OpenAPI naming.
type ChatCompletionResponse = ChatResponse

// ChatStreamMessageEvent represents the data payload for an SSE message event.
type ChatStreamMessageEvent struct {
	Delta string `json:"delta"`
}

// ChatStreamDoneEvent represents the data payload for an SSE done event.
type ChatStreamDoneEvent struct {
	TotalTokens int   `json:"total_tokens"`
	DurationMs  int64 `json:"duration_ms"`
}
