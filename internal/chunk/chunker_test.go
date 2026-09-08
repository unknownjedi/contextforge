package chunk_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/your-org/contextforge/internal/chunk"
)

func TestChunker_GoSourceLinePreservation(t *testing.T) {
	code := `package main

import "fmt"

// FirstFunction calculates sum
func FirstFunction(a, b int) int {
	return a + b
}

// SecondFunction calculates product
func SecondFunction(x, y int) int {
	result := x * y
	return result
}
`

	chunker := chunk.NewChunker(chunk.ChunkerOptions{
		TargetTokens:  20, // Intentionally small to force multi-chunk split
		OverlapTokens: 5,
		MaxLines:      8,
	})

	chunks := chunker.ChunkText(code, "go")
	require.NotEmpty(t, chunks)

	// Verify line preservation
	lines := strings.Split(strings.TrimSuffix(code, "\n"), "\n")
	for _, c := range chunks {
		assert.GreaterOrEqual(t, c.StartLine, 1)
		assert.LessOrEqual(t, c.EndLine, len(lines))
		assert.True(t, c.StartLine <= c.EndLine)

		// Check that the chunk text matches lines from source
		expectedText := strings.Join(lines[c.StartLine-1:c.EndLine], "\n")
		assert.Equal(t, expectedText, c.Content)
		assert.NotEmpty(t, c.ContentHash)
		assert.Greater(t, c.TokenCount, 0)
	}

	// First chunk must start at line 1
	assert.Equal(t, 1, chunks[0].StartLine)
	// Last chunk must end at last line
	assert.Equal(t, len(lines), chunks[len(chunks)-1].EndLine)
}

func TestChunker_MarkdownHeadings(t *testing.T) {
	md := `# Overview
ContextForge is a high performance RAG engine.

## Installation
Run make dev to get started.

## Configuration
Configure your .env file with appropriate keys.
`

	chunker := chunk.NewChunker(chunk.ChunkerOptions{
		TargetTokens:  15,
		OverlapTokens: 3,
		MaxLines:      5,
	})

	chunks := chunker.ChunkText(md, "markdown")
	require.NotEmpty(t, chunks)

	for _, c := range chunks {
		assert.NotEmpty(t, c.Content)
		assert.NotEmpty(t, c.ContentHash)
	}
}

func TestChunker_EmptyAndWhitespace(t *testing.T) {
	chunker := chunk.NewChunker(chunk.DefaultOptions())

	assert.Nil(t, chunker.ChunkText("", "go"))
	assert.Nil(t, chunker.ChunkText("   \n\n\t  ", "go"))
}

func TestEstimateTokens(t *testing.T) {
	assert.Equal(t, 0, chunk.EstimateTokens(""))
	assert.Greater(t, chunk.EstimateTokens("func main() { fmt.Println(\"Hello\") }"), 5)
}
