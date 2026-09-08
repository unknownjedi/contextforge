package retrieval_test

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/repository"
	"github.com/your-org/contextforge/internal/retrieval"
)

func TestReciprocalRankFusion_Math(t *testing.T) {
	t.Run("Default k=60 single ranking score calculation", func(t *testing.T) {
		id1 := uuid.New()
		id2 := uuid.New()

		ranking := []model.ChunkMatch{
			{
				Chunk:      &model.DocumentChunk{ID: id1},
				Similarity: 0.9,
			},
			{
				Chunk:      &model.DocumentChunk{ID: id2},
				Similarity: 0.8,
			},
		}

		fused := retrieval.ReciprocalRankFusion([][]model.ChunkMatch{ranking}, 0) // default k=60
		require.Len(t, fused, 2)

		// Expected rank 1: 1 / (60 + 1) = 1/61
		expectedScore1 := float32(1.0 / 61.0)
		assert.Equal(t, id1, fused[0].Chunk.ID)
		assert.InDelta(t, expectedScore1, fused[0].Score, 1e-6)

		// Expected rank 2: 1 / (60 + 2) = 1/62
		expectedScore2 := float32(1.0 / 62.0)
		assert.Equal(t, id2, fused[1].Chunk.ID)
		assert.InDelta(t, expectedScore2, fused[1].Score, 1e-6)
	})

	t.Run("Custom k multi-ranking score combination", func(t *testing.T) {
		idA := uuid.New()
		idB := uuid.New()
		idC := uuid.New()

		k := 10

		// Ranking 1: [A (rank 1), B (rank 2)]
		r1 := []model.ChunkMatch{
			{Chunk: &model.DocumentChunk{ID: idA}, Similarity: 0.85},
			{Chunk: &model.DocumentChunk{ID: idB}, Similarity: 0.70},
		}

		// Ranking 2: [B (rank 1), C (rank 2), A (rank 3)]
		r2 := []model.ChunkMatch{
			{Chunk: &model.DocumentChunk{ID: idB}, Similarity: 0.70},
			{Chunk: &model.DocumentChunk{ID: idC}, Similarity: 0.60},
			{Chunk: &model.DocumentChunk{ID: idA}, Similarity: 0.85},
		}

		fused := retrieval.ReciprocalRankFusion([][]model.ChunkMatch{r1, r2}, k)
		require.Len(t, fused, 3)

		// Expected scores with k=10:
		// A: 1/(10+1) + 1/(10+3) = 1/11 + 1/13 = 0.090909 + 0.076923 = 0.167832
		// B: 1/(10+2) + 1/(10+1) = 1/12 + 1/11 = 0.083333 + 0.090909 = 0.174242
		// C: 1/(10+2) = 1/12 = 0.083333
		expectedA := float32(1.0/11.0 + 1.0/13.0)
		expectedB := float32(1.0/12.0 + 1.0/11.0)
		expectedC := float32(1.0 / 12.0)

		// B has highest score, so should be 1st
		assert.Equal(t, idB, fused[0].Chunk.ID)
		assert.InDelta(t, expectedB, fused[0].Score, 1e-6)

		// A has second highest score
		assert.Equal(t, idA, fused[1].Chunk.ID)
		assert.InDelta(t, expectedA, fused[1].Score, 1e-6)

		// C has third score
		assert.Equal(t, idC, fused[2].Chunk.ID)
		assert.InDelta(t, expectedC, fused[2].Score, 1e-6)
	})

	t.Run("Empty or invalid rankings returns empty slice", func(t *testing.T) {
		assert.Empty(t, retrieval.ReciprocalRankFusion(nil, 60))
		assert.Empty(t, retrieval.ReciprocalRankFusion([][]model.ChunkMatch{}, 60))
		assert.Empty(t, retrieval.ReciprocalRankFusion([][]model.ChunkMatch{{}}, 60))
	})

	t.Run("Duplicate items within same ranking only counted once", func(t *testing.T) {
		id1 := uuid.New()
		ranking := []model.ChunkMatch{
			{Chunk: &model.DocumentChunk{ID: id1}, Similarity: 0.9},
			{Chunk: &model.DocumentChunk{ID: id1}, Similarity: 0.9}, // Duplicate
		}

		fused := retrieval.ReciprocalRankFusion([][]model.ChunkMatch{ranking}, 60)
		require.Len(t, fused, 1)
		assert.InDelta(t, float32(1.0/61.0), fused[0].Score, 1e-6)
	})
}

func TestHybridRetriever_Retrieve(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	otherProjectID := uuid.New()

	vectorRepo := repository.NewMockVectorRepository()
	embedProvider := provider.NewMockEmbeddingProvider(128)

	// Seed chunks for testing
	docID := uuid.New()

	// Chunk 1: General authentication overview
	chunk1 := &model.DocumentChunk{
		ID:          uuid.New(),
		ProjectID:   projectID,
		DocumentID:  docID,
		ChunkIndex:  0,
		StartLine:   1,
		EndLine:     20,
		Content:     "package auth\n\nfunc AuthenticateUser(token string) bool {\n\treturn token != \"\"\n}",
		TokenCount:  25,
		ContentHash: "hash-1",
	}

	// Chunk 2: Encryption details with specific function EncryptAES256
	chunk2 := &model.DocumentChunk{
		ID:          uuid.New(),
		ProjectID:   projectID,
		DocumentID:  docID,
		ChunkIndex:  1,
		StartLine:   21,
		EndLine:     50,
		Content:     "package crypto\n\n// EncryptAES256 encrypts payload using Galois/Counter Mode\nfunc EncryptAES256(key, plaintext []byte) ([]byte, error) {\n\treturn nil, nil\n}",
		TokenCount:  30,
		ContentHash: "hash-2",
	}

	// Chunk 3: Belongs to other project (project isolation test)
	chunk3 := &model.DocumentChunk{
		ID:          uuid.New(),
		ProjectID:   otherProjectID,
		DocumentID:  uuid.New(),
		ChunkIndex:  0,
		StartLine:   1,
		EndLine:     15,
		Content:     "package secret\n\nfunc EncryptAES256() string {\n\treturn \"leak\"\n}",
		TokenCount:  20,
		ContentHash: "hash-3",
	}

	// Generate embeddings for the chunks
	embeds, err := embedProvider.EmbedDocuments(ctx, []string{chunk1.Content, chunk2.Content, chunk3.Content})
	require.NoError(t, err)

	chunk1.Embedding = embeds[0]
	chunk2.Embedding = embeds[1]
	chunk3.Embedding = embeds[2]

	err = vectorRepo.UpsertChunks(ctx, []*model.DocumentChunk{chunk1, chunk2, chunk3})
	require.NoError(t, err)

	retriever := retrieval.NewHybridRetriever(vectorRepo, embedProvider)

	t.Run("Project isolation: other project chunks are never retrieved", func(t *testing.T) {
		results, err := retriever.Retrieve(ctx, projectID, "EncryptAES256", 5, 0, nil)
		require.NoError(t, err)

		for _, r := range results {
			assert.Equal(t, projectID, r.Chunk.ProjectID)
			assert.NotEqual(t, otherProjectID, r.Chunk.ProjectID)
		}
	})

	t.Run("Exact keyword query boosts matching chunk", func(t *testing.T) {
		results, err := retriever.Retrieve(ctx, projectID, "EncryptAES256 Galois/Counter", 5, 0, nil)
		require.NoError(t, err)
		require.NotEmpty(t, results)

		// The chunk containing exact function name EncryptAES256 should be top ranked
		assert.Equal(t, chunk2.ID, results[0].Chunk.ID)
		assert.Contains(t, results[0].Chunk.Content, "EncryptAES256")
		assert.Greater(t, results[0].Score, float32(0))
	})

	t.Run("Empty query returns empty slice without error", func(t *testing.T) {
		results, err := retriever.Retrieve(ctx, projectID, "", 5, 0, nil)
		require.NoError(t, err)
		assert.Empty(t, results)

		results, err = retriever.Retrieve(ctx, projectID, "   ", 5, 0, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("Nil project ID returns validation error", func(t *testing.T) {
		_, err := retriever.Retrieve(ctx, uuid.Nil, "test", 5, 0, nil)
		assert.Error(t, err)
	})

	t.Run("File filter matches specified path", func(t *testing.T) {
		// Create a custom mock vector repo that preserves file paths on matches
		customMockRepo := &mockRepoWithPaths{
			MockVectorRepository: repository.NewMockVectorRepository(),
			filePaths: map[uuid.UUID]string{
				chunk1.ID: "internal/auth/auth.go",
				chunk2.ID: "internal/crypto/encrypt.go",
			},
		}
		require.NoError(t, customMockRepo.UpsertChunks(ctx, []*model.DocumentChunk{chunk1, chunk2}))

		customRetriever := retrieval.NewHybridRetriever(customMockRepo, embedProvider)

		// Filter for crypto path
		results, err := customRetriever.Retrieve(ctx, projectID, "AuthenticateUser EncryptAES256", 5, 0, []string{"internal/crypto/*"})
		require.NoError(t, err)

		for _, r := range results {
			assert.Contains(t, r.FilePath, "internal/crypto")
		}
	})

	t.Run("Zero vector results returns empty slice", func(t *testing.T) {
		emptyRepo := repository.NewMockVectorRepository()
		emptyRetriever := retrieval.NewHybridRetriever(emptyRepo, embedProvider)

		results, err := emptyRetriever.Retrieve(ctx, projectID, "nonexistent term", 5, 0, nil)
		require.NoError(t, err)
		assert.Empty(t, results)
	})

	t.Run("Query with regex characters does not panic or error", func(t *testing.T) {
		results, err := retriever.Retrieve(ctx, projectID, "func(a, b *[]string) + [0-9]?", 5, 0, nil)
		require.NoError(t, err)
		assert.NotNil(t, results)
	})
}

// mockRepoWithPaths wraps MockVectorRepository to inject FilePath and SourceID into results.
type mockRepoWithPaths struct {
	*repository.MockVectorRepository
	filePaths map[uuid.UUID]string
}

func (m *mockRepoWithPaths) SearchSimilar(ctx context.Context, params model.VectorSearchParams) ([]*model.ChunkMatch, error) {
	matches, err := m.MockVectorRepository.SearchSimilar(ctx, params)
	if err != nil {
		return nil, err
	}
	for _, match := range matches {
		if path, ok := m.filePaths[match.Chunk.ID]; ok {
			match.FilePath = path
		}
	}
	return matches, nil
}

// Ensure unused math import does not trigger compiler error
var _ = math.Abs
