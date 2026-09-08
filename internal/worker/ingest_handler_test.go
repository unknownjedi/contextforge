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
