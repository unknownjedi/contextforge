package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/repository"
)

func TestMockVectorRepository_BasicOperations(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewMockVectorRepository()

	projectA := uuid.New()
	projectB := uuid.New()
	docA1 := uuid.New()
	docB1 := uuid.New()

	// 1. Insert chunks into Project A
	chunksA := []*model.DocumentChunk{
		{
			ID:          uuid.New(),
			ProjectID:   projectA,
			DocumentID:  docA1,
			ChunkIndex:  0,
			StartLine:   1,
			EndLine:     20,
			Content:     "func HandleAuth() { ... }",
			ContentHash: "hash_auth",
			TokenCount:  50,
			Embedding:   []float32{1.0, 0.0, 0.0},
			CreatedAt:   time.Now(),
		},
		{
			ID:          uuid.New(),
			ProjectID:   projectA,
			DocumentID:  docA1,
			ChunkIndex:  1,
			StartLine:   21,
			EndLine:     40,
			Content:     "func HandleDatabase() { ... }",
			ContentHash: "hash_db",
			TokenCount:  60,
			Embedding:   []float32{0.0, 1.0, 0.0},
			CreatedAt:   time.Now(),
		},
	}

	// 2. Insert chunk into Project B
	chunksB := []*model.DocumentChunk{
		{
			ID:          uuid.New(),
			ProjectID:   projectB,
			DocumentID:  docB1,
			ChunkIndex:  0,
			StartLine:   1,
			EndLine:     15,
			Content:     "Project B secret algorithm",
			ContentHash: "hash_secret",
			TokenCount:  45,
			Embedding:   []float32{1.0, 0.0, 0.0}, // Identical embedding to Project A chunk!
			CreatedAt:   time.Now(),
		},
	}

	err := repo.UpsertChunks(ctx, chunksA)
	require.NoError(t, err)

	err = repo.UpsertChunks(ctx, chunksB)
	require.NoError(t, err)

	// Verify counts
	countA, err := repo.CountChunksByProjectID(ctx, projectA)
	require.NoError(t, err)
	assert.Equal(t, int64(2), countA)

	countB, err := repo.CountChunksByProjectID(ctx, projectB)
	require.NoError(t, err)
	assert.Equal(t, int64(1), countB)

	// 3. Search Project A with query embedding [1.0, 0.0, 0.0]
	matches, err := repo.SearchSimilar(ctx, model.VectorSearchParams{
		ProjectID:      projectA,
		QueryEmbedding: []float32{1.0, 0.0, 0.0},
		TopK:           5,
		SimilarityMin:  0.5,
	})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, chunksA[0].ID, matches[0].Chunk.ID)
	assert.InDelta(t, float32(1.0), matches[0].Similarity, 0.001)

	// Verify Project B chunk was NOT leaked even though it has similarity 1.0
	for _, m := range matches {
		assert.Equal(t, projectA, m.Chunk.ProjectID)
		assert.NotEqual(t, projectB, m.Chunk.ProjectID)
	}

	// 4. Delete chunks by document
	err = repo.DeleteChunksByDocumentID(ctx, projectA, docA1)
	require.NoError(t, err)

	countAAfterDel, err := repo.CountChunksByProjectID(ctx, projectA)
	require.NoError(t, err)
	assert.Equal(t, int64(0), countAAfterDel)

	// Project B must remain untouched
	countBAfterDel, err := repo.CountChunksByProjectID(ctx, projectB)
	require.NoError(t, err)
	assert.Equal(t, int64(1), countBAfterDel)
}
