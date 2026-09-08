package model

// ChatMessage represents a single message in a chat conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Common chat message roles.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// CompletionRequest encapsulates parameters for a text/chat completion.
type CompletionRequest struct {
	Messages    []ChatMessage `json:"messages"`
	Model       string        `json:"model"`
	Temperature float32       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
}

// CompletionResponse represents the result of a completion generation.
type CompletionResponse struct {
	Content    string `json:"content"`
	Model      string `json:"model"`
	TokensUsed int    `json:"tokens_used,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// StreamChunk represents an incremental token or text segment from a streaming completion.
type StreamChunk struct {
	Delta      string `json:"delta"`
	Done       bool   `json:"done"`
	TokensUsed int    `json:"tokens_used,omitempty"`
}

// EmbeddingRequest encapsulates parameters for generating vector embeddings.
type EmbeddingRequest struct {
	Texts []string `json:"texts"`
	Model string   `json:"model"`
}

// EmbeddingResponse contains the generated vector embeddings and metadata.
type EmbeddingResponse struct {
	Embeddings  [][]float32 `json:"embeddings"`
	Model       string      `json:"model"`
	TotalTokens int         `json:"total_tokens,omitempty"`
}
