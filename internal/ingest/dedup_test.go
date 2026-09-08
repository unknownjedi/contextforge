package ingest_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/your-org/contextforge/internal/ingest"
	"github.com/your-org/contextforge/internal/model"
)

func TestDeduplication_EmbeddingReuse(t *testing.T) {
	ctx := context.Background()
	cache := ingest.NewMemoryDedupCache()

	projectA := uuid.New()
	projectB := uuid.New()

	hashShared := "sha256_shared_code_function"
	hashUnique := "sha256_unique_code_snippet"
	embedding := []float32{0.25, 0.5, 0.75}

	// 1. Prime cache with embedding in Project A
	cache.PutEmbedding(ctx, projectA, hashShared, embedding)

	// 2. Chunks to process in Project A
	chunksA := []*model.DocumentChunk{
		{
			ID:          uuid.New(),
			ProjectID:   projectA,
			ContentHash: hashShared,
		},
		{
			ID:          uuid.New(),
			ProjectID:   projectA,
			ContentHash: hashUnique,
		},
	}

	resultA := ingest.FilterDuplicateChunks(ctx, cache, projectA, chunksA)
	assert.Equal(t, 1, resultA.EmbeddingsSaved)
	assert.Len(t, resultA.CachedChunks, 1)
	assert.Len(t, resultA.ChunksToEmbed, 1)
	assert.Equal(t, embedding, resultA.CachedChunks[0].Embedding)

	// 3. Project B with the same hash should NOT reuse Project A's cache (Strict isolation)
	chunksB := []*model.DocumentChunk{
		{
			ID:          uuid.New(),
			ProjectID:   projectB,
			ContentHash: hashShared,
		},
	}

	resultB := ingest.FilterDuplicateChunks(ctx, cache, projectB, chunksB)
	assert.Equal(t, 0, resultB.EmbeddingsSaved, "Project B should not reuse Project A cached embedding")
	assert.Len(t, resultB.ChunksToEmbed, 1)
	assert.Len(t, resultB.CachedChunks, 0)
}
