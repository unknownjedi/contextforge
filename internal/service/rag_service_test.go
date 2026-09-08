package service_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/repository"
	"github.com/your-org/contextforge/internal/retrieval"
	"github.com/your-org/contextforge/internal/service"
)

func setupTestRAGService(t *testing.T, projectID uuid.UUID) (*service.RAGService, *repository.MockVectorRepository, *provider.MockLLMProvider, *provider.MockEmbeddingProvider) {
	ctx := context.Background()

	vectorRepo := repository.NewMockVectorRepository()
	embedProvider := provider.NewMockEmbeddingProvider(64)
	llmProvider := provider.NewMockLLMProvider("ContextForge encrypts tokens using AES-256-GCM [1].")

	// Seed chunks for testing
	docID := uuid.New()
	chunk := &model.DocumentChunk{
		ID:          uuid.New(),
		ProjectID:   projectID,
		DocumentID:  docID,
		ChunkIndex:  0,
		StartLine:   12,
		EndLine:     35,
		Content:     "func EncryptToken(token string) ([]byte, error) {\n\t// AES-256-GCM encryption\n}",
		TokenCount:  30,
		ContentHash: "hash-encrypt",
	}

	embeds, err := embedProvider.EmbedDocuments(ctx, []string{chunk.Content})
	require.NoError(t, err)
	chunk.Embedding = embeds[0]

	err = vectorRepo.UpsertChunks(ctx, []*model.DocumentChunk{chunk})
	require.NoError(t, err)

	hybridRetriever := retrieval.NewHybridRetriever(vectorRepo, embedProvider)
	ragService := service.NewRAGService(hybridRetriever, llmProvider)

	return ragService, vectorRepo, llmProvider, embedProvider
}

func TestRAGService_Generate(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ragSvc, _, llmProvider, _ := setupTestRAGService(t, projectID)

	t.Run("Synchronous Generate returns answer, citations, and metrics", func(t *testing.T) {
		req := &model.ChatRequest{
			Message: "How are tokens encrypted?",
			TopK:    5,
		}

		resp, err := ragSvc.Generate(ctx, projectID, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Contains(t, resp.Answer, "AES-256-GCM")
		require.Len(t, resp.Citations, 1)
		assert.Equal(t, 12, resp.Citations[0].StartLine)
		assert.Equal(t, 35, resp.Citations[0].EndLine)
		assert.Contains(t, resp.Citations[0].Snippet, "EncryptToken")
		assert.Greater(t, resp.TokensUsed, 0)
		assert.GreaterOrEqual(t, resp.DurationMs, int64(0))
	})

	t.Run("System prompt passed to LLM contains citations instruction and context", func(t *testing.T) {
		var capturedReq *model.CompletionRequest
		llmProvider.CustomGenerate = func(ctx context.Context, r *model.CompletionRequest) (*model.CompletionResponse, error) {
			capturedReq = r
			return &model.CompletionResponse{
				Content:    "Mock response",
				Model:      r.Model,
				TokensUsed: 15,
			}, nil
		}

		req := &model.ChatRequest{
			Message: "Where is encryption defined?",
		}

		_, err := ragSvc.Generate(ctx, projectID, req)
		require.NoError(t, err)
		require.NotNil(t, capturedReq)
		require.Len(t, capturedReq.Messages, 2)

		systemMsg := capturedReq.Messages[0].Content
		assert.Contains(t, systemMsg, "ContextForge AI")
		assert.Contains(t, systemMsg, "lines 12-35")
		assert.Contains(t, systemMsg, "EncryptToken")

		userMsg := capturedReq.Messages[1].Content
		assert.Equal(t, "Where is encryption defined?", userMsg)
	})

	t.Run("Validation errors for empty message or nil project ID", func(t *testing.T) {
		_, err := ragSvc.Generate(ctx, uuid.Nil, &model.ChatRequest{Message: "test"})
		assert.Error(t, err)

		_, err = ragSvc.Generate(ctx, projectID, &model.ChatRequest{Message: ""})
		assert.Error(t, err)

		_, err = ragSvc.Generate(ctx, projectID, nil)
		assert.Error(t, err)
	})
}

func TestRAGService_StreamChat(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ragSvc, _, _, _ := setupTestRAGService(t, projectID)

	t.Run("Streams citations first, then tokens in order", func(t *testing.T) {
		var mu sync.Mutex
		var eventSequence []string
		var capturedCitations []model.Citation
		var capturedTokens []string

		onCitation := func(c *model.Citation) error {
			mu.Lock()
			defer mu.Unlock()
			eventSequence = append(eventSequence, "citation")
			capturedCitations = append(capturedCitations, *c)
			return nil
		}

		onToken := func(token string) error {
			mu.Lock()
			defer mu.Unlock()
			eventSequence = append(eventSequence, "token")
			capturedTokens = append(capturedTokens, token)
			return nil
		}

		req := &model.ChatRequest{
			Message: "Tell me about encryption",
			TopK:    5,
		}

		resp, err := ragSvc.StreamChat(ctx, projectID, req, onCitation, onToken)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Verify citations were delivered before tokens
		require.NotEmpty(t, capturedCitations)
		require.NotEmpty(t, capturedTokens)

		citationCount := len(capturedCitations)
		for i := 0; i < citationCount; i++ {
			assert.Equal(t, "citation", eventSequence[i])
		}
		for i := citationCount; i < len(eventSequence); i++ {
			assert.Equal(t, "token", eventSequence[i])
		}

		// Verify accumulated answer matches streamed tokens
		streamedText := strings.Join(capturedTokens, "")
		assert.Contains(t, resp.Answer, "AES-256-GCM")
		assert.Equal(t, resp.Answer, streamedText)
		assert.Equal(t, len(capturedCitations), len(resp.Citations))
	})

	t.Run("LLM error terminates streaming with error", func(t *testing.T) {
		failRetriever := &mockFailingRetriever{err: errors.New("retrieval failed")}
		failLLM := provider.NewMockLLMProvider("ok")
		failSvc := service.NewRAGService(failRetriever, failLLM)

		_, err := failSvc.StreamChat(ctx, projectID, &model.ChatRequest{Message: "fail query"}, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "retrieval failed")
	})
}

func TestRAGService_ProjectIsolation(t *testing.T) {
	ctx := context.Background()
	projectA := uuid.New()
	projectB := uuid.New()

	vectorRepo := repository.NewMockVectorRepository()
	embedProvider := provider.NewMockEmbeddingProvider(64)
	llmProvider := provider.NewMockLLMProvider("Answer")

	// Store chunk in Project A only
	chunkA := &model.DocumentChunk{
		ID:          uuid.New(),
		ProjectID:   projectA,
		DocumentID:  uuid.New(),
		ChunkIndex:  0,
		StartLine:   1,
		EndLine:     10,
		Content:     "package auth\nfunc PrivateKeyA() string { return \"secretA\" }",
		TokenCount:  20,
		ContentHash: "hash-a",
	}
	embeds, _ := embedProvider.EmbedDocuments(ctx, []string{chunkA.Content})
	chunkA.Embedding = embeds[0]
	require.NoError(t, vectorRepo.UpsertChunks(ctx, []*model.DocumentChunk{chunkA}))

	hybridRetriever := retrieval.NewHybridRetriever(vectorRepo, embedProvider)
	ragSvc := service.NewRAGService(hybridRetriever, llmProvider)

	// Query from Project B
	resp, err := ragSvc.Generate(ctx, projectB, &model.ChatRequest{
		Message: "PrivateKeyA secretA",
	})
	require.NoError(t, err)

	// Project B must receive zero citations from Project A
	assert.Empty(t, resp.Citations)
}

type mockFailingRetriever struct {
	err error
}

func (m *mockFailingRetriever) Retrieve(ctx context.Context, projectID uuid.UUID, query string, topK int, simMin float32, fileFilters []string) ([]*model.ChunkMatch, error) {
	return nil, m.err
}
