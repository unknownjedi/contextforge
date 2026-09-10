package provider

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"strings"
	"time"

	"github.com/your-org/contextforge/internal/model"
)

// MockLLMProvider is a mock implementation of LLMProvider for testing.
type MockLLMProvider struct {
	ResponseContent string
	ModelName       string
	TokensUsed      int
	Delay           time.Duration
	CustomGenerate  func(ctx context.Context, req *model.CompletionRequest) (*model.CompletionResponse, error)
	CustomStream    func(ctx context.Context, req *model.CompletionRequest, onChunk func(chunk *model.StreamChunk) error) (*model.CompletionResponse, error)
}

// NewMockLLMProvider creates a mock LLM provider.
func NewMockLLMProvider(defaultResponse string) *MockLLMProvider {
	if defaultResponse == "" {
		defaultResponse = "mock response"
	}
	return &MockLLMProvider{
		ResponseContent: defaultResponse,
		ModelName:       "mock-model",
		TokensUsed:      42,
	}
}

// GenerateCompletion returns the mock response.
func (m *MockLLMProvider) GenerateCompletion(ctx context.Context, req *model.CompletionRequest) (*model.CompletionResponse, error) {
	if m.CustomGenerate != nil {
		return m.CustomGenerate(ctx, req)
	}
	if m.Delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.Delay):
		}
	}
	modelName := m.ModelName
	if req.Model != "" {
		modelName = req.Model
	}
	return &model.CompletionResponse{
		Content:    m.ResponseContent,
		Model:      modelName,
		TokensUsed: m.TokensUsed,
		DurationMs: 1,
	}, nil
}

// StreamCompletion splits the response into word chunks.
func (m *MockLLMProvider) StreamCompletion(ctx context.Context, req *model.CompletionRequest, onChunk func(chunk *model.StreamChunk) error) (*model.CompletionResponse, error) {
	if m.CustomStream != nil {
		return m.CustomStream(ctx, req, onChunk)
	}

	modelName := m.ModelName
	if req.Model != "" {
		modelName = req.Model
	}

	words := strings.Fields(m.ResponseContent)
	if len(words) == 0 {
		words = []string{m.ResponseContent}
	}

	var full strings.Builder
	for i, w := range words {
		if i > 0 {
			w = " " + w
		}
		full.WriteString(w)
		if onChunk != nil {
			if err := onChunk(&model.StreamChunk{
				Delta:      w,
				Done:       false,
				TokensUsed: i + 1,
			}); err != nil {
				return nil, err
			}
		}
		if m.Delay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(m.Delay):
			}
		}
	}

	if onChunk != nil {
		if err := onChunk(&model.StreamChunk{
			Delta:      "",
			Done:       true,
			TokensUsed: m.TokensUsed,
		}); err != nil {
			return nil, err
		}
	}

	return &model.CompletionResponse{
		Content:    m.ResponseContent,
		Model:      modelName,
		TokensUsed: m.TokensUsed,
		DurationMs: 1,
	}, nil
}

// MockEmbeddingProvider is a mock implementation of EmbeddingProvider for testing.
type MockEmbeddingProvider struct {
	DimensionValue int
	ModelNameValue string
	CustomEmbed    func(ctx context.Context, texts []string) ([][]float32, error)
}

// NewMockEmbeddingProvider creates a mock embedding provider.
func NewMockEmbeddingProvider(dimension int) *MockEmbeddingProvider {
	if dimension <= 0 {
		dimension = 768
	}
	return &MockEmbeddingProvider{
		DimensionValue: dimension,
		ModelNameValue: "mock-embed-model",
	}
}

// Dimension returns the mock dimension.
func (m *MockEmbeddingProvider) Dimension() int {
	return m.DimensionValue
}

// ModelName returns the mock model name.
func (m *MockEmbeddingProvider) ModelName() string {
	return m.ModelNameValue
}

// EmbedDocuments generates deterministic mock embeddings.
func (m *MockEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if m.CustomEmbed != nil {
		return m.CustomEmbed(ctx, texts)
	}

	results := make([][]float32, len(texts))
	for i, text := range texts {
		results[i] = generateDeterministicVector(text, m.DimensionValue)
	}
	return results, nil
}

// EmbedBatch generates deterministic mock embeddings (alias for EmbedDocuments).
func (m *MockEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return m.EmbedDocuments(ctx, texts)
}

// EmbedQuery returns an embedding for a single text query.
func (m *MockEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vecs, err := m.EmbedDocuments(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

// generateDeterministicVector produces a normalized vector of specified dimension using SHA-256 hash.
func generateDeterministicVector(text string, dim int) []float32 {
	vec := make([]float32, dim)
	h := sha256.Sum256([]byte(text))
	seed := binary.BigEndian.Uint64(h[:8])

	var sumSq float64
	for i := 0; i < dim; i++ {
		// Linear congruential generator step
		seed = seed*6364136223846793005 + 1442695040888963407
		val := float32(float64(int64(seed)) / float64(math.MaxInt64))
		vec[i] = val
		sumSq += float64(val * val)
	}

	norm := float32(math.Sqrt(sumSq))
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}

	return vec
}
