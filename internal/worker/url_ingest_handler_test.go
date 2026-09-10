package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/chunk"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/queue"
	"github.com/your-org/contextforge/internal/repository"
	"github.com/your-org/contextforge/internal/urlfetch"
	"github.com/your-org/contextforge/internal/worker"
)

type mockURLFetcher struct {
	result *urlfetch.FetchResult
	err    error
}

func (m *mockURLFetcher) Fetch(ctx context.Context, targetURL string) (*urlfetch.FetchResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func TestURLIngestHandler_EndToEnd(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	sourceID := uuid.New()
	jobID := uuid.New()
	targetURL := "https://docs.contextforge.dev/quickstart"

	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)

	fetchResult := &urlfetch.FetchResult{
		URL:   targetURL,
		Title: "ContextForge Quickstart",
		Content: `# Quickstart Guide

ContextForge connects to your codebases and documentation repositories.

## Step 1: Add a Source
Add a URL or GitHub repository to begin context indexing.

## Step 2: Query Context
Perform hybrid semantic search across tokenized chunks.`,
		ContentType: "text/markdown",
		StatusCode:  200,
		ContentHash: "hash_quickstart_123",
	}

	handler := worker.NewURLIngestHandler(
		nil, // sourceRepo optional in test
		nil, // docRepo optional in test
		vectorRepo,
		nil, // jobRepo optional in test
		embedder,
		chunk.NewChunker(chunk.DefaultOptions()),
		&mockURLFetcher{result: fetchResult},
		zap.NewNop(),
	)

	payloadBytes, err := json.Marshal(queue.URLSyncPayload{
		JobID:     jobID,
		ProjectID: projectID,
		SourceID:  sourceID,
		URL:       targetURL,
	})
	require.NoError(t, err)

	task := asynq.NewTask(queue.TypeURLSync, payloadBytes)
	err = handler.ProcessURLSyncTask(ctx, task)
	require.NoError(t, err)

	// Verify chunks indexed in vector repository
	count, err := vectorRepo.CountChunksByProjectID(ctx, projectID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(1), "should have at least 1 chunk indexed")

	// Verify similarity search retrieval
	matches, err := vectorRepo.SearchSimilar(ctx, model.VectorSearchParams{
		ProjectID:      projectID,
		QueryEmbedding: make([]float32, 768),
		TopK:           5,
		SimilarityMin:  0.0,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, matches)
	assert.Equal(t, projectID, matches[0].Chunk.ProjectID)
	assert.Contains(t, matches[0].Chunk.Content, "Quickstart")
}

func TestURLIngestHandler_FetchError(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	sourceID := uuid.New()
	jobID := uuid.New()
	targetURL := "https://example.com/notfound"

	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)

	fetchErr := errors.New("connection timed out")
	handler := worker.NewURLIngestHandler(
		nil,
		nil,
		vectorRepo,
		nil,
		embedder,
		chunk.NewChunker(chunk.DefaultOptions()),
		&mockURLFetcher{err: fetchErr},
		zap.NewNop(),
	)

	payloadBytes, err := json.Marshal(queue.URLSyncPayload{
		JobID:     jobID,
		ProjectID: projectID,
		SourceID:  sourceID,
		URL:       targetURL,
	})
	require.NoError(t, err)

	task := asynq.NewTask(queue.TypeURLSync, payloadBytes)
	err = handler.ProcessURLSyncTask(ctx, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection timed out")
}

func TestURLIngestHandler_EmbedderError(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	sourceID := uuid.New()
	jobID := uuid.New()
	targetURL := "https://example.com/docs"

	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	embedder.CustomEmbed = func(ctx context.Context, texts []string) ([][]float32, error) {
		return nil, errors.New("embedding quota exceeded")
	}

	fetchResult := &urlfetch.FetchResult{
		URL:         targetURL,
		Title:       "Test Docs",
		Content:     "# Title\n\nSome documentation text.",
		ContentType: "text/markdown",
		StatusCode:  200,
		ContentHash: "hash_test_123",
	}

	handler := worker.NewURLIngestHandler(
		nil,
		nil,
		vectorRepo,
		nil,
		embedder,
		chunk.NewChunker(chunk.DefaultOptions()),
		&mockURLFetcher{result: fetchResult},
		zap.NewNop(),
	)

	payloadBytes, err := json.Marshal(queue.URLSyncPayload{
		JobID:     jobID,
		ProjectID: projectID,
		SourceID:  sourceID,
		URL:       targetURL,
	})
	require.NoError(t, err)

	task := asynq.NewTask(queue.TypeURLSync, payloadBytes)
	err = handler.ProcessURLSyncTask(ctx, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "embedding quota exceeded")
}

func TestURLIngestHandler_MissingURL(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	sourceID := uuid.New()
	jobID := uuid.New()

	handler := worker.NewURLIngestHandler(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		&mockURLFetcher{},
		zap.NewNop(),
	)

	payloadBytes, err := json.Marshal(queue.URLSyncPayload{
		JobID:     jobID,
		ProjectID: projectID,
		SourceID:  sourceID,
		URL:       "", // Empty URL
	})
	require.NoError(t, err)

	task := asynq.NewTask(queue.TypeURLSync, payloadBytes)
	err = handler.ProcessURLSyncTask(ctx, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}
