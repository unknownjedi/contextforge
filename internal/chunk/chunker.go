package chunk

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"
)

// Chunk represents a chunk of source text with exact 1-indexed line coordinates.
type Chunk struct {
	Index       int    `json:"index"`
	StartLine   int    `json:"start_line"` // 1-indexed, inclusive
	EndLine     int    `json:"end_line"`   // 1-indexed, inclusive
	Content     string `json:"content"`
	ContentHash string `json:"content_hash"`
	TokenCount  int    `json:"token_count"`
}

// ChunkerOptions configures the code and text chunking parameters.
type ChunkerOptions struct {
	TargetTokens  int // Target chunk size in tokens (default 384)
	OverlapTokens int // Overlap between consecutive chunks in tokens (default 40)
	MaxLines      int // Maximum lines per chunk fallback (default 100)
}

// DefaultOptions returns recommended chunking settings for code and docs.
func DefaultOptions() ChunkerOptions {
	return ChunkerOptions{
		TargetTokens:  384,
		OverlapTokens: 40,
		MaxLines:      100,
	}
}

// MultiLanguageChunker splits code and documentation files into line-anchored semantic chunks.
type MultiLanguageChunker struct {
	options ChunkerOptions
}

// NewChunker initializes a new multi-language chunker.
func NewChunker(opts ChunkerOptions) *MultiLanguageChunker {
	if opts.TargetTokens <= 0 {
		opts.TargetTokens = 384
	}
	if opts.OverlapTokens < 0 || opts.OverlapTokens >= opts.TargetTokens {
		opts.OverlapTokens = 40
	}
	if opts.MaxLines <= 0 {
		opts.MaxLines = 100
	}
	return &MultiLanguageChunker{options: opts}
}

// EstimateTokens calculates an approximate token count based on whitespace and sub-word boundaries (~4 chars per token).
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	words := 0
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			words++
		}
	}
	// Code and technical text typically has ~1.3 tokens per whitespace-separated word
	tokens := int(float64(words) * 1.3)
	if tokens == 0 && len(text) > 0 {
		return 1
	}
	return tokens
}

// HashContent returns the hex SHA-256 digest of the chunk text.
func HashContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// ChunkText splits source code or markdown into line-anchored chunks.
func (c *MultiLanguageChunker) ChunkText(content string, language string) []Chunk {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	// 1. Break content into lines preserving exact line indexing without 64KB bufio.Scanner limit
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(strings.TrimSuffix(normalized, "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{content}
	}

	var chunks []Chunk
	lineCount := len(lines)
	startIdx := 0

	for startIdx < lineCount {
		currentTokens := 0
		endIdx := startIdx

		// Accumulate lines until target tokens or max lines is reached
		for endIdx < lineCount {
			lineTokens := EstimateTokens(lines[endIdx])
			if lineTokens == 0 {
				lineTokens = 1 // Newline cost
			}

			if currentTokens+lineTokens > c.options.TargetTokens && ((endIdx-startIdx) >= 5 || currentTokens >= c.options.TargetTokens*2) {
				// We hit target token threshold, try to break on structural boundary if possible
				if isStructuralBoundary(lines[endIdx], language) || (endIdx-startIdx) >= c.options.MaxLines || currentTokens >= c.options.TargetTokens*2 {
					break
				}
			}

			currentTokens += lineTokens
			endIdx++

			if (endIdx - startIdx) >= c.options.MaxLines {
				break
			}
		}

		if endIdx == startIdx {
			endIdx = startIdx + 1
		}

		// Extract chunk slice
		chunkLines := lines[startIdx:endIdx]
		chunkContent := strings.Join(chunkLines, "\n")
		chunkTokens := EstimateTokens(chunkContent)

		chunk := Chunk{
			Index:       len(chunks),
			StartLine:   startIdx + 1, // 1-indexed
			EndLine:     endIdx,       // 1-indexed, inclusive
			Content:     chunkContent,
			ContentHash: HashContent(chunkContent),
			TokenCount:  chunkTokens,
		}
		chunks = append(chunks, chunk)

		if endIdx >= lineCount {
			break
		}

		// Calculate sliding overlap in lines
		overlapLines := 0
		overlapTokens := 0
		for i := endIdx - 1; i >= startIdx; i-- {
			t := EstimateTokens(lines[i])
			if overlapTokens+t > c.options.OverlapTokens {
				break
			}
			overlapTokens += t
			overlapLines++
		}

		// Advance startIdx, ensuring forward progress
		nextStart := endIdx - overlapLines
		if nextStart <= startIdx {
			nextStart = startIdx + 1
		}
		startIdx = nextStart
	}

	return chunks
}

// isStructuralBoundary detects function definitions, classes, headings, or block openings.
func isStructuralBoundary(line string, language string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return true // Empty line is a great chunk boundary
	}

	switch strings.ToLower(language) {
	case "go":
		return strings.HasPrefix(trimmed, "func ") ||
			strings.HasPrefix(trimmed, "type ") ||
			strings.HasPrefix(trimmed, "const ") ||
			strings.HasPrefix(trimmed, "var ")
	case "python", "py":
		return strings.HasPrefix(trimmed, "def ") ||
			strings.HasPrefix(trimmed, "class ") ||
			strings.HasPrefix(trimmed, "@")
	case "typescript", "javascript", "ts", "js", "tsx", "jsx":
		return strings.HasPrefix(trimmed, "export ") ||
			strings.HasPrefix(trimmed, "function ") ||
			strings.HasPrefix(trimmed, "class ") ||
			strings.HasPrefix(trimmed, "const ") ||
			strings.HasPrefix(trimmed, "interface ") ||
			strings.HasPrefix(trimmed, "type ")
	case "markdown", "md":
		return strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "---")
	default:
		return strings.HasPrefix(trimmed, "func ") ||
			strings.HasPrefix(trimmed, "def ") ||
			strings.HasPrefix(trimmed, "class ") ||
			strings.HasPrefix(trimmed, "#")
	}
}
