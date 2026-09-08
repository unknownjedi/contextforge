package rag_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/rag"
)

func TestExtractCitations(t *testing.T) {
	sourceID1 := uuid.New()
	sourceID2 := uuid.New()
	docID := uuid.New()

	matches := []*model.ChunkMatch{
		{
			SourceID:   sourceID1,
			FilePath:   "internal/crypto/encrypt.go",
			Similarity: 0.884,
			Language:   "go",
			Chunk: &model.DocumentChunk{
				ID:         uuid.New(),
				DocumentID: docID,
				StartLine:  12,
				EndLine:    35,
				Content:    "func Encrypt(key, plaintext []byte) ([]byte, error) {\n\t// AES-GCM implementation\n}",
			},
		},
		{
			SourceID:   sourceID2,
			FilePath:   "internal/auth/jwt.go",
			Similarity: 0.765,
			Language:   "go",
			Chunk: &model.DocumentChunk{
				ID:         uuid.New(),
				DocumentID: docID,
				StartLine:  42,
				EndLine:    68,
				Content:    "func GenerateToken(userID uuid.UUID) (string, error) {\n\t// token generation\n}",
			},
		},
	}

	t.Run("Successfully extracts citations preserving line anchors and metadata", func(t *testing.T) {
		citations := rag.ExtractCitations(matches)
		require.Len(t, citations, 2)

		// Check Citation 1
		assert.Equal(t, sourceID1, citations[0].SourceID)
		assert.Equal(t, "internal/crypto/encrypt.go", citations[0].FilePath)
		assert.Equal(t, 12, citations[0].StartLine)
		assert.Equal(t, 35, citations[0].EndLine)
		assert.InDelta(t, float32(0.884), citations[0].Similarity, 1e-4)
		assert.Equal(t, "func Encrypt(key, plaintext []byte) ([]byte, error) {\n\t// AES-GCM implementation\n}", citations[0].Snippet)

		// Check Citation 2
		assert.Equal(t, sourceID2, citations[1].SourceID)
		assert.Equal(t, "internal/auth/jwt.go", citations[1].FilePath)
		assert.Equal(t, 42, citations[1].StartLine)
		assert.Equal(t, 68, citations[1].EndLine)
		assert.InDelta(t, float32(0.765), citations[1].Similarity, 1e-4)
		assert.Contains(t, citations[1].Snippet, "GenerateToken")
	})

	t.Run("Empty matches returns non-nil empty slice", func(t *testing.T) {
		citations := rag.ExtractCitations([]*model.ChunkMatch{})
		assert.NotNil(t, citations)
		assert.Empty(t, citations)

		citations = rag.ExtractCitations(nil)
		assert.NotNil(t, citations)
		assert.Empty(t, citations)
	})

	t.Run("Fall back to DocumentID if SourceID is nil", func(t *testing.T) {
		fallbackDocID := uuid.New()
		single := []*model.ChunkMatch{
			{
				SourceID:   uuid.Nil,
				FilePath:   "pkg/test.go",
				Similarity: 0.9,
				Chunk: &model.DocumentChunk{
					DocumentID: fallbackDocID,
					StartLine:  1,
					EndLine:    10,
					Content:    "package test",
				},
			},
		}

		citations := rag.ExtractCitations(single)
		require.Len(t, citations, 1)
		assert.Equal(t, fallbackDocID, citations[0].SourceID)
	})
}

func TestFormatContextPrompt(t *testing.T) {
	matches := []*model.ChunkMatch{
		{
			FilePath: "internal/crypto/encrypt.go",
			Language: "go",
			Chunk: &model.DocumentChunk{
				StartLine: 12,
				EndLine:   35,
				Content:   "func Encrypt(...) {\n\t// AES-256-GCM\n}",
			},
		},
		{
			FilePath: "internal/auth/jwt.go",
			Language: "go",
			Chunk: &model.DocumentChunk{
				StartLine: 42,
				EndLine:   68,
				Content:   "func ValidateToken(...) bool {\n\treturn true\n}",
			},
		},
	}

	t.Run("Formats prompt with numbered references [1], [2], file paths, and line ranges", func(t *testing.T) {
		prompt := rag.FormatContextPrompt(matches)

		// Verify numbered references
		assert.Contains(t, prompt, "[1] Source: internal/crypto/encrypt.go (lines 12-35)")
		assert.Contains(t, prompt, "func Encrypt(...)")
		assert.Contains(t, prompt, "```go")

		assert.Contains(t, prompt, "[2] Source: internal/auth/jwt.go (lines 42-68)")
		assert.Contains(t, prompt, "func ValidateToken(...)")
	})

	t.Run("Empty matches returns empty string", func(t *testing.T) {
		assert.Equal(t, "", rag.FormatContextPrompt(nil))
		assert.Equal(t, "", rag.FormatContextPrompt([]*model.ChunkMatch{}))
	})

	t.Run("BuildSystemPrompt injects context and hallucination safeguards", func(t *testing.T) {
		contextStr := rag.FormatContextPrompt(matches)
		systemPrompt := rag.BuildSystemPrompt(contextStr)

		assert.Contains(t, systemPrompt, "ContextForge AI")
		assert.Contains(t, systemPrompt, "Never invent or hallucinate citations")
		assert.Contains(t, systemPrompt, "untrusted project code")
		assert.Contains(t, systemPrompt, "[1] Source: internal/crypto/encrypt.go (lines 12-35)")
	})
}
