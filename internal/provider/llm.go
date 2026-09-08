package provider

import (
	"context"

	"github.com/your-org/contextforge/internal/model"
)

// LLMProvider defines the unified interface for LLM completions.
type LLMProvider interface {
	// GenerateCompletion performs a non-streaming completion request.
	GenerateCompletion(ctx context.Context, req *model.CompletionRequest) (*model.CompletionResponse, error)

	// StreamCompletion performs a streaming completion request, invoking onChunk for each incremental token/delta.
	// When finished, it returns the accumulated CompletionResponse or an error.
	StreamCompletion(ctx context.Context, req *model.CompletionRequest, onChunk func(chunk *model.StreamChunk) error) (*model.CompletionResponse, error)
}
