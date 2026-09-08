package provider

import (
	"context"
)

// OpenAIEmbeddingProvider implements EmbeddingProvider specifically for OpenAI embedding models.
type OpenAIEmbeddingProvider struct {
	client    *OpenAIProvider
	dimension int
	modelName string
}

// NewOpenAIEmbeddingProvider creates an instance of OpenAIEmbeddingProvider.
func NewOpenAIEmbeddingProvider(cfg OpenAIConfig) *OpenAIEmbeddingProvider {
	client := NewOpenAIProvider(cfg)
	modelName := cfg.DefaultEmbeddingModel
	if modelName == "" {
		modelName = DefaultOpenAIEmbeddingModel
	}

	dim := 1536
	if modelName == "text-embedding-3-large" {
		dim = 3072
	}

	return &OpenAIEmbeddingProvider{
		client:    client,
		dimension: dim,
		modelName: modelName,
	}
}

// EmbedDocuments generates embeddings for multiple documents.
func (p *OpenAIEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return p.client.EmbedDocuments(ctx, texts)
}

// EmbedQuery generates an embedding for a single text query.
func (p *OpenAIEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return p.client.EmbedQuery(ctx, text)
}

// Dimension returns the vector dimension of the embedding model.
func (p *OpenAIEmbeddingProvider) Dimension() int {
	return p.dimension
}

// ModelName returns the configured embedding model name.
func (p *OpenAIEmbeddingProvider) ModelName() string {
	return p.modelName
}
