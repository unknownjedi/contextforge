package worker_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/your-org/contextforge/internal/chunk"
	"github.com/your-org/contextforge/internal/ent"
	"github.com/your-org/contextforge/internal/ingest"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/queue"
	"github.com/your-org/contextforge/internal/repository"
	"github.com/your-org/contextforge/internal/worker"
)

type mockFetcher struct {
	files []*ingest.ScannedFile
}

func (m *mockFetcher) FetchFiles(ctx context.Context, source *ent.Source) ([]*ingest.ScannedFile, error) {
	return m.files, nil
}

func TestIngestionPipeline_EndToEnd(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	sourceID := uuid.New()
	jobID := uuid.New()

	// Setup mock vector repo & mock embedding provider
	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)

	files := []*ingest.ScannedFile{
		{
			Path:        "main.go",
			Language:    "go",
			Content:     "package main\n\nfunc main() {\n\tprintln(\"hello world\")\n}\n",
			ContentHash: "hash_main_123",
			SizeBytes:   50,
		},
		{
			Path:        "README.md",
			Language:    "markdown",
			Content:     "# ContextForge\n\nHigh performance context platform.\n",
			ContentHash: "hash_readme_123",
			SizeBytes:   50,
		},
	}

	fetcher := &mockFetcher{files: files}
	chunker := chunk.NewChunker(chunk.DefaultOptions())
	dedupCache := ingest.NewMemoryDedupCache()

	pipeline := worker.NewIngestionPipeline(
		nil, // docRepo optional in unit test
		vectorRepo,
		nil, // jobRepo optional in unit test
		embedder,
		chunker,
		dedupCache,
		fetcher,
		zap.NewNop(),
	)

	payload := queue.RepoSyncPayload{
		JobID:     jobID,
		ProjectID: projectID,
		SourceID:  sourceID,
	}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	task := asynq.NewTask(queue.TypeRepoSync, payloadBytes)

	// Execute ingestion pipeline
	err = pipeline.ProcessSyncTask(ctx, task)
	require.NoError(t, err)

	// Assert chunks were indexed in vector repo
	count, err := vectorRepo.CountChunksByProjectID(ctx, projectID)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(2), "should have at least 2 chunks indexed")

	// Search vector repo to verify retrieval
	matches, err := vectorRepo.SearchSimilar(ctx, model.VectorSearchParams{
		ProjectID:      projectID,
		QueryEmbedding: make([]float32, 768),
		TopK:           10,
		SimilarityMin:  0.0,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, matches)

	for _, m := range matches {
		assert.Equal(t, projectID, m.Chunk.ProjectID)
		assert.NotEmpty(t, m.Chunk.Content)
		assert.Greater(t, m.Chunk.StartLine, 0)
		assert.GreaterOrEqual(t, m.Chunk.EndLine, m.Chunk.StartLine)
	}
}

func TestIngestionPipeline_EmbedderError(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	sourceID := uuid.New()
	jobID := uuid.New()

	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	embedder.CustomEmbed = func(ctx context.Context, texts []string) ([][]float32, error) {
		return nil, assert.AnError
	}

	files := []*ingest.ScannedFile{
		{
			Path:        "main.go",
			Language:    "go",
			Content:     "package main\nfunc main() {}",
			ContentHash: "hash_fail",
			SizeBytes:   25,
		},
	}

	pipeline := worker.NewIngestionPipeline(
		nil,
		vectorRepo,
		nil,
		embedder,
		chunk.NewChunker(chunk.DefaultOptions()),
		ingest.NewMemoryDedupCache(),
		&mockFetcher{files: files},
		zap.NewNop(),
	)

	payloadBytes, err := json.Marshal(queue.RepoSyncPayload{
		JobID:     jobID,
		ProjectID: projectID,
		SourceID:  sourceID,
	})
	require.NoError(t, err)

	task := asynq.NewTask(queue.TypeRepoSync, payloadBytes)
	err = pipeline.ProcessSyncTask(ctx, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "generating embeddings")
}

func TestIngestionPipeline_EmbedderCountMismatch(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	sourceID := uuid.New()
	jobID := uuid.New()

	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)
	// Return fewer embeddings than requested
	embedder.CustomEmbed = func(ctx context.Context, texts []string) ([][]float32, error) {
		return [][]float32{}, nil
	}

	files := []*ingest.ScannedFile{
		{
			Path:        "main.go",
			Language:    "go",
			Content:     "package main\nfunc main() {}",
			ContentHash: "hash_mismatch",
			SizeBytes:   25,
		},
	}

	pipeline := worker.NewIngestionPipeline(
		nil,
		vectorRepo,
		nil,
		embedder,
		chunk.NewChunker(chunk.DefaultOptions()),
		ingest.NewMemoryDedupCache(),
		&mockFetcher{files: files},
		zap.NewNop(),
	)

	payloadBytes, err := json.Marshal(queue.RepoSyncPayload{
		JobID:     jobID,
		ProjectID: projectID,
		SourceID:  sourceID,
	})
	require.NoError(t, err)

	task := asynq.NewTask(queue.TypeRepoSync, payloadBytes)
	err = pipeline.ProcessSyncTask(ctx, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "embedding count mismatch")
}

func TestIngestionPipeline_NilEmbedder(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	sourceID := uuid.New()
	jobID := uuid.New()

	vectorRepo := repository.NewMockVectorRepository()

	files := []*ingest.ScannedFile{
		{
			Path:        "main.go",
			Language:    "go",
			Content:     "package main\nfunc main() {}",
			ContentHash: "hash_no_embedder",
			SizeBytes:   25,
		},
	}

	pipeline := worker.NewIngestionPipeline(
		nil,
		vectorRepo,
		nil,
		nil, // Nil embedder
		chunk.NewChunker(chunk.DefaultOptions()),
		ingest.NewMemoryDedupCache(),
		&mockFetcher{files: files},
		zap.NewNop(),
	)

	payloadBytes, err := json.Marshal(queue.RepoSyncPayload{
		JobID:     jobID,
		ProjectID: projectID,
		SourceID:  sourceID,
	})
	require.NoError(t, err)

	task := asynq.NewTask(queue.TypeRepoSync, payloadBytes)
	err = pipeline.ProcessSyncTask(ctx, task)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "embedding provider required")
}

func TestIngestionPipeline_EmptyAndBinaryFiltering(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	sourceID := uuid.New()
	jobID := uuid.New()

	vectorRepo := repository.NewMockVectorRepository()
	embedder := provider.NewMockEmbeddingProvider(768)

	files := []*ingest.ScannedFile{
		{
			Path:        "empty.go",
			Language:    "go",
			Content:     "   \n\n\t  ",
			ContentHash: "hash_empty",
			SizeBytes:   10,
		},
		{
			Path:        "binary.bin",
			Language:    "binary",
			Content:     "binary\x00data\x00here",
			ContentHash: "hash_binary",
			SizeBytes:   15,
		},
	}

	pipeline := worker.NewIngestionPipeline(
		nil,
		vectorRepo,
		nil,
		embedder,
		chunk.NewChunker(chunk.DefaultOptions()),
		ingest.NewMemoryDedupCache(),
		&mockFetcher{files: files},
		zap.NewNop(),
	)

	payloadBytes, err := json.Marshal(queue.RepoSyncPayload{
		JobID:     jobID,
		ProjectID: projectID,
		SourceID:  sourceID,
	})
	require.NoError(t, err)

	task := asynq.NewTask(queue.TypeRepoSync, payloadBytes)
	err = pipeline.ProcessSyncTask(ctx, task)
	require.NoError(t, err)

	count, err := vectorRepo.CountChunksByProjectID(ctx, projectID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count, "empty and binary files should not be indexed")
}

func TestIngestionPipeline_NilDedupCacheDefaults(t *testing.T) {
	pipeline := worker.NewIngestionPipeline(
		nil, nil, nil, nil, nil, nil, nil, zap.NewNop(),
	)
	assert.NotNil(t, pipeline)
}
