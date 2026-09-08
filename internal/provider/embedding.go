package provider

import "context"

// EmbeddingProvider defines the unified interface for generating vector embeddings.
type EmbeddingProvider interface {
	// EmbedDocuments generates vector embeddings for a slice of text strings.
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)

	// EmbedQuery generates a vector embedding for a single text query.
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}
