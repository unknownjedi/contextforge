package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/your-org/contextforge/internal/model"
)

const (
	DefaultOllamaBaseURL   = "http://localhost:11434"
	DefaultOllamaModel     = "nomic-embed-text"
	DefaultOllamaDimension = 768
)

// OllamaConfig configures the Ollama embedding provider.
type OllamaConfig struct {
	BaseURL    string
	Model      string
	Dimension  int
	HTTPClient *http.Client
}

// OllamaEmbeddingProvider implements EmbeddingProvider for Ollama local instances.
type OllamaEmbeddingProvider struct {
	baseURL    string
	model      string
	dimension  int
	httpClient *http.Client
}

// NewOllamaEmbeddingProvider creates a new OllamaEmbeddingProvider.
func NewOllamaEmbeddingProvider(cfg OllamaConfig) *OllamaEmbeddingProvider {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultOllamaBaseURL
	}
	modelName := cfg.Model
	if modelName == "" {
		modelName = DefaultOllamaModel
	}
	dim := cfg.Dimension
	if dim <= 0 {
		dim = DefaultOllamaDimension
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	return &OllamaEmbeddingProvider{
		baseURL:    baseURL,
		model:      modelName,
		dimension:  dim,
		httpClient: client,
	}
}

// Dimension returns the embedding dimension size.
func (p *OllamaEmbeddingProvider) Dimension() int {
	return p.dimension
}

// ModelName returns the configured model name.
func (p *OllamaEmbeddingProvider) ModelName() string {
	return p.model
}

type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
}

type ollamaLegacyEmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaLegacyEmbeddingResponse struct {
	Embedding []float32 `json:"embedding"`
}

// EmbedDocuments generates embeddings for a slice of texts.
func (p *OllamaEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// Try the modern /api/embed batch endpoint first
	endpoint := fmt.Sprintf("%s/api/embed", p.baseURL)
	payload := ollamaEmbedRequest{
		Model: p.model,
		Input: texts,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ollama embed request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	// If /api/embed is supported and returns 200
	if resp.StatusCode == http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read ollama response: %w", err)
		}

		var embedResp ollamaEmbedResponse
		if err := json.Unmarshal(respBody, &embedResp); err != nil {
			return nil, fmt.Errorf("failed to decode ollama response: %w", err)
		}
		return embedResp.Embeddings, nil
	}

	// If /api/embed returned 404, fallback to legacy /api/embeddings endpoint
	if resp.StatusCode == http.StatusNotFound {
		return p.embedLegacyBatch(ctx, texts)
	}

	respBody, _ := io.ReadAll(resp.Body)
	return nil, fmt.Errorf("ollama error (status %d): %s", resp.StatusCode, string(respBody))
}

// embedLegacyBatch calls /api/embeddings sequentially for older Ollama versions.
func (p *OllamaEmbeddingProvider) embedLegacyBatch(ctx context.Context, texts []string) ([][]float32, error) {
	legacyEndpoint := fmt.Sprintf("%s/api/embeddings", p.baseURL)
	results := make([][]float32, len(texts))

	for i, text := range texts {
		payload := ollamaLegacyEmbeddingRequest{
			Model:  p.model,
			Prompt: text,
		}

		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal legacy ollama request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, legacyEndpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to create legacy ollama request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("legacy ollama request failed: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read legacy ollama response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("legacy ollama api error (status %d): %s", resp.StatusCode, string(respBody))
		}

		var legacyResp ollamaLegacyEmbeddingResponse
		if err := json.Unmarshal(respBody, &legacyResp); err != nil {
			return nil, fmt.Errorf("failed to decode legacy ollama response: %w", err)
		}
		results[i] = legacyResp.Embedding
	}

	return results, nil
}

// EmbedQuery generates an embedding for a single text query.
func (p *OllamaEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	res, err := p.EmbedDocuments(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, fmt.Errorf("ollama returned empty embedding")
	}
	return res[0], nil
}

// EmbedRequest generates embeddings given a model.EmbeddingRequest.
func (p *OllamaEmbeddingProvider) Embed(ctx context.Context, req *model.EmbeddingRequest) (*model.EmbeddingResponse, error) {
	embeddings, err := p.EmbedDocuments(ctx, req.Texts)
	if err != nil {
		return nil, err
	}
	return &model.EmbeddingResponse{
		Embeddings: embeddings,
		Model:      p.model,
	}, nil
}

// EmbedBatch generates embeddings for multiple documents (alias for EmbedDocuments).
func (p *OllamaEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	return p.EmbedDocuments(ctx, texts)
}
