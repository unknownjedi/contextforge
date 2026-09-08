package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/your-org/contextforge/internal/model"
)

// -----------------------------------------------------------------------------
// Ollama Embedding Tests
// -----------------------------------------------------------------------------

func TestOllamaEmbeddingProvider_ModernEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/embed" {
			t.Errorf("unexpected method or path: %s %s", r.Method, r.URL.Path)
		}

		var reqBody ollamaEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if reqBody.Model != "nomic-embed-text" {
			t.Errorf("expected model nomic-embed-text, got %s", reqBody.Model)
		}

		var embeddings [][]float32
		for i := range reqBody.Input {
			if i == 0 {
				embeddings = append(embeddings, []float32{0.1, 0.2, 0.3})
			} else {
				embeddings = append(embeddings, []float32{0.4, 0.5, 0.6})
			}
		}

		resp := ollamaEmbedResponse{
			Model:      "nomic-embed-text",
			Embeddings: embeddings,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOllamaEmbeddingProvider(OllamaConfig{
		BaseURL:    server.URL,
		Model:      "nomic-embed-text",
		Dimension:  768,
		HTTPClient: server.Client(),
	})

	if provider.Dimension() != 768 {
		t.Errorf("expected dim 768, got %d", provider.Dimension())
	}
	if provider.ModelName() != "nomic-embed-text" {
		t.Errorf("expected nomic-embed-text, got %s", provider.ModelName())
	}

	embeddings, err := provider.EmbedDocuments(context.Background(), []string{"doc 1", "doc 2"})
	if err != nil {
		t.Fatalf("EmbedDocuments failed: %v", err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}
	if embeddings[0][0] != 0.1 || embeddings[1][0] != 0.4 {
		t.Errorf("unexpected embeddings content: %v", embeddings)
	}

	// Test EmbedQuery
	queryEmb, err := provider.EmbedQuery(context.Background(), "doc 1")
	if err != nil {
		t.Fatalf("EmbedQuery failed: %v", err)
	}
	if len(queryEmb) != 3 {
		t.Errorf("expected 3 values, got %d", len(queryEmb))
	}

	// Test Embed wrapper
	embedResp, err := provider.Embed(context.Background(), &model.EmbeddingRequest{
		Texts: []string{"doc 1"},
	})
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(embedResp.Embeddings) != 1 {
		t.Errorf("expected 1 embedding, got %d", len(embedResp.Embeddings))
	}
}

func TestOllamaEmbeddingProvider_LegacyFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Modern /api/embed returns 404
		if r.URL.Path == "/api/embed" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Legacy /api/embeddings endpoint
		if r.URL.Path == "/api/embeddings" {
			var reqBody ollamaLegacyEmbeddingRequest
			_ = json.NewDecoder(r.Body).Decode(&reqBody)

			resp := ollamaLegacyEmbeddingResponse{
				Embedding: []float32{0.7, 0.8, 0.9},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	provider := NewOllamaEmbeddingProvider(OllamaConfig{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	})

	embeddings, err := provider.EmbedDocuments(context.Background(), []string{"test 1", "test 2"})
	if err != nil {
		t.Fatalf("expected legacy fallback to succeed, got: %v", err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}
	if embeddings[0][0] != 0.7 {
		t.Errorf("expected 0.7, got %f", embeddings[0][0])
	}
}

func TestOllamaEmbeddingProvider_Empty(t *testing.T) {
	provider := NewOllamaEmbeddingProvider(OllamaConfig{})
	res, err := provider.EmbedDocuments(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error on empty: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("expected 0 embeddings, got %d", len(res))
	}
}

// -----------------------------------------------------------------------------
// OpenAI Embedding Tests
// -----------------------------------------------------------------------------

func TestOpenAIEmbeddingProvider(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/embeddings" {
			t.Errorf("unexpected method or path: %s %s", r.Method, r.URL.Path)
		}
		callCount++

		var reqBody openAIEmbeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		var data []openAIEmbeddingData
		for i := range reqBody.Input {
			data = append(data, openAIEmbeddingData{
				Index:     i,
				Embedding: []float32{float32(i) * 0.1, 0.2, 0.3},
			})
		}

		resp := openAIEmbeddingResponse{
			Data:  data,
			Model: reqBody.Model,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOpenAIEmbeddingProvider(OpenAIConfig{
		APIKey:                "sk-test",
		BaseURL:               server.URL,
		DefaultEmbeddingModel: "text-embedding-3-small",
		HTTPClient:            server.Client(),
		Retry:                 fastRetryConfig(),
	})

	if provider.Dimension() != 1536 {
		t.Errorf("expected dim 1536, got %d", provider.Dimension())
	}
	if provider.ModelName() != "text-embedding-3-small" {
		t.Errorf("expected model text-embedding-3-small, got %s", provider.ModelName())
	}

	embeddings, err := provider.EmbedDocuments(context.Background(), []string{"query 1", "query 2"})
	if err != nil {
		t.Fatalf("EmbedDocuments failed: %v", err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}

	// Test single query
	qVec, err := provider.EmbedQuery(context.Background(), "single")
	if err != nil {
		t.Fatalf("EmbedQuery failed: %v", err)
	}
	if len(qVec) != 3 {
		t.Errorf("expected 3 values, got %d", len(qVec))
	}
}

func TestOpenAIEmbeddingProvider_LargeBatchSplitting(t *testing.T) {
	batchCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		batchCount++

		var reqBody openAIEmbeddingRequest
		_ = json.NewDecoder(r.Body).Decode(&reqBody)

		var data []openAIEmbeddingData
		for i := range reqBody.Input {
			data = append(data, openAIEmbeddingData{
				Index:     i,
				Embedding: []float32{0.5},
			})
		}

		resp := openAIEmbeddingResponse{
			Data:  data,
			Model: reqBody.Model,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewOpenAIEmbeddingProvider(OpenAIConfig{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Retry:      fastRetryConfig(),
	})

	// Generate 1200 inputs (batch size is 500, so this requires 3 batches: 500 + 500 + 200)
	inputs := make([]string, 1200)
	for i := range inputs {
		inputs[i] = fmt.Sprintf("chunk %d", i)
	}

	embeddings, err := provider.EmbedDocuments(context.Background(), inputs)
	if err != nil {
		t.Fatalf("EmbedDocuments failed: %v", err)
	}
	if len(embeddings) != 1200 {
		t.Fatalf("expected 1200 embeddings, got %d", len(embeddings))
	}
	if batchCount != 3 {
		t.Errorf("expected 3 batches for 1200 inputs, got %d batches", batchCount)
	}
}

// -----------------------------------------------------------------------------
// Gemini Embedding Tests
// -----------------------------------------------------------------------------

func TestGeminiEmbeddingProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "batchEmbedContents") {
			t.Errorf("expected batchEmbedContents in path, got %s", r.URL.Path)
		}

		var reqBody geminiBatchEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode gemini embed request: %v", err)
		}

		var embeddings []geminiEmbedValues
		for i := range reqBody.Requests {
			embeddings = append(embeddings, geminiEmbedValues{
				Values: []float32{float32(i) * 0.2, 0.5, 0.8},
			})
		}

		resp := geminiBatchEmbedResponse{Embeddings: embeddings}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewGeminiProvider(GeminiConfig{
		APIKey:                "test-gemini-key",
		BaseURL:               server.URL,
		DefaultEmbeddingModel: "text-embedding-004",
		HTTPClient:            server.Client(),
		Retry:                 fastRetryConfig(),
	})

	embeddings, err := provider.EmbedDocuments(context.Background(), []string{"gemini 1", "gemini 2"})
	if err != nil {
		t.Fatalf("EmbedDocuments failed: %v", err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}

	qVec, err := provider.EmbedQuery(context.Background(), "query")
	if err != nil {
		t.Fatalf("EmbedQuery failed: %v", err)
	}
	if len(qVec) != 3 {
		t.Errorf("expected 3 elements in query embedding, got %d", len(qVec))
	}
}
