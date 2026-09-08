package retrieval

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/model"
)

// DefaultRRFK defines the standard reciprocal rank fusion smoothing parameter.
const DefaultRRFK = 60

// ReciprocalRankFusion merges multiple ranked lists of ChunkMatch items into a single unified
// ranking ordered by Reciprocal Rank Fusion (RRF) score.
// The score is computed as: \sum \frac{1}{k + rank}, where rank is 1-based (1, 2, 3, ...).
// If k <= 0, DefaultRRFK (60) is used.
func ReciprocalRankFusion(rankings [][]model.ChunkMatch, k int) []*model.ChunkMatch {
	if k <= 0 {
		k = DefaultRRFK
	}

	scores := make(map[string]float32)
	matchMap := make(map[string]*model.ChunkMatch)
	order := make([]string, 0)

	for _, ranking := range rankings {
		seenInRanking := make(map[string]bool)
		for i, item := range ranking {
			key := chunkMatchKey(&item)
			if key == "" || seenInRanking[key] {
				continue
			}
			seenInRanking[key] = true

			rank := i + 1
			reciprocalRank := float32(1.0 / float64(k+rank))
			scores[key] += reciprocalRank

			existing, ok := matchMap[key]
			if !ok {
				copied := item
				matchMap[key] = &copied
				order = append(order, key)
			} else {
				// Retain highest similarity across lists
				if item.Similarity > existing.Similarity {
					existing.Similarity = item.Similarity
				}
				if existing.SourceID == uuid.Nil && item.SourceID != uuid.Nil {
					existing.SourceID = item.SourceID
				}
				if existing.FilePath == "" && item.FilePath != "" {
					existing.FilePath = item.FilePath
				}
				if existing.Language == "" && item.Language != "" {
					existing.Language = item.Language
				}
			}
		}
	}

	results := make([]*model.ChunkMatch, 0, len(matchMap))
	for _, key := range order {
		m := matchMap[key]
		m.Score = scores[key]
		results = append(results, m)
	}

	// Sort descending by RRF score; tie-break deterministically
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Similarity != results[j].Similarity {
			return results[i].Similarity > results[j].Similarity
		}
		return chunkMatchKey(results[i]) < chunkMatchKey(results[j])
	})

	return results
}

// chunkMatchKey extracts a unique identifier for a ChunkMatch candidate.
func chunkMatchKey(m *model.ChunkMatch) string {
	if m == nil {
		return ""
	}
	if m.Chunk != nil && m.Chunk.ID != uuid.Nil {
		return m.Chunk.ID.String()
	}
	if m.FilePath != "" && m.Chunk != nil {
		return fmt.Sprintf("%s:%d:%d", m.FilePath, m.Chunk.StartLine, m.Chunk.EndLine)
	}
	if m.Chunk != nil && m.Chunk.ContentHash != "" {
		return m.Chunk.ContentHash
	}
	if m.Chunk != nil && m.Chunk.Content != "" {
		return m.Chunk.Content
	}
	if m.FilePath != "" {
		return m.FilePath
	}
	return ""
}
