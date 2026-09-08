package retrieval

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/model"
	"github.com/your-org/contextforge/internal/provider"
	"github.com/your-org/contextforge/internal/repository"
)

// Retriever defines the contract for contextual search across project knowledge.
type Retriever interface {
	Retrieve(ctx context.Context, projectID uuid.UUID, query string, topK int, simMin float32, fileFilters []string) ([]*model.ChunkMatch, error)
}

// HybridRetriever combines dense vector similarity search with sparse lexical keyword matching,
// fusing rankings via Reciprocal Rank Fusion (RRF).
type HybridRetriever struct {
	vectorRepo        repository.VectorRepository
	embeddingProvider provider.EmbeddingProvider
	rrfK              int
}

// NewHybridRetriever constructs a HybridRetriever with default RRF smoothing.
func NewHybridRetriever(vectorRepo repository.VectorRepository, embeddingProvider provider.EmbeddingProvider) *HybridRetriever {
	return &HybridRetriever{
		vectorRepo:        vectorRepo,
		embeddingProvider: embeddingProvider,
		rrfK:              DefaultRRFK,
	}
}

// SetRRFK configures the reciprocal rank fusion constant (default 60).
func (r *HybridRetriever) SetRRFK(k int) {
	if k > 0 {
		r.rrfK = k
	}
}

// Retrieve searches for relevant chunks strictly scoped to projectID, combining vector
// similarity and lexical keyword matches through Reciprocal Rank Fusion (RRF).
func (r *HybridRetriever) Retrieve(
	ctx context.Context,
	projectID uuid.UUID,
	query string,
	topK int,
	simMin float32,
	fileFilters []string,
) ([]*model.ChunkMatch, error) {
	if projectID == uuid.Nil {
		return nil, errors.New("project ID is required")
	}

	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return []*model.ChunkMatch{}, nil
	}

	if topK <= 0 {
		topK = 5
	}

	// 1. Generate dense vector embedding for the query
	queryEmbedding, err := r.embeddingProvider.EmbedQuery(ctx, trimmedQuery)
	if err != nil {
		return nil, fmt.Errorf("generating query embedding: %w", err)
	}

	// 2. Query vector repository for top candidates
	// We retrieve a broader candidate pool to enable effective fusion with lexical ranking
	candidateLimit := topK * 4
	if candidateLimit < 20 {
		candidateLimit = 20
	}

	params := model.VectorSearchParams{
		ProjectID:      projectID,
		QueryEmbedding: queryEmbedding,
		TopK:           candidateLimit,
		SimilarityMin:  simMin,
		FileFilters:    fileFilters,
	}

	vectorCandidates, err := r.vectorRepo.SearchSimilar(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("executing vector search: %w", err)
	}

	if len(vectorCandidates) == 0 {
		return []*model.ChunkMatch{}, nil
	}

	// 3. Perform lexical / keyword scoring on candidate chunk contents
	lexicalCandidates := r.scoreLexicalMatches(trimmedQuery, vectorCandidates)

	// 4. Build ranking lists for RRF
	vectorRanking := make([]model.ChunkMatch, len(vectorCandidates))
	for i, m := range vectorCandidates {
		vectorRanking[i] = *m
	}

	lexicalRanking := make([]model.ChunkMatch, len(lexicalCandidates))
	for i, m := range lexicalCandidates {
		lexicalRanking[i] = *m
	}

	// 5. Fuse rankings using Reciprocal Rank Fusion
	rankings := [][]model.ChunkMatch{vectorRanking}
	if len(lexicalRanking) > 0 {
		rankings = append(rankings, lexicalRanking)
	}

	fused := ReciprocalRankFusion(rankings, r.rrfK)

	// 6. Apply post-filtering for fileFilters (if repository does not natively handle them)
	filtered := make([]*model.ChunkMatch, 0, len(fused))
	for _, m := range fused {
		if len(fileFilters) > 0 && m.FilePath != "" {
			if !matchesAnyFileFilter(m.FilePath, fileFilters) {
				continue
			}
		}
		filtered = append(filtered, m)
	}

	// 7. Limit to topK requested
	if len(filtered) > topK {
		filtered = filtered[:topK]
	}

	return filtered, nil
}

// scoreLexicalMatches computes lexical keyword relevance scores for candidates and returns
// a sorted slice descending by lexical relevance score.
func (r *HybridRetriever) scoreLexicalMatches(query string, candidates []*model.ChunkMatch) []*model.ChunkMatch {
	lowerQuery := strings.ToLower(query)
	terms := extractTerms(lowerQuery)

	type scoredMatch struct {
		match *model.ChunkMatch
		score float32
	}

	scored := make([]scoredMatch, 0, len(candidates))

	for _, cand := range candidates {
		if cand.Chunk == nil {
			continue
		}

		content := cand.Chunk.Content
		lowerContent := strings.ToLower(content)
		lowerFilePath := strings.ToLower(cand.FilePath)

		var score float32

		// Exact phrase boost
		if len(lowerQuery) > 3 && strings.Contains(lowerContent, lowerQuery) {
			score += 10.0
		}

		// Keyword frequency & token coverage
		matchedTerms := 0
		for _, term := range terms {
			if len(term) < 2 {
				continue
			}

			count := strings.Count(lowerContent, term)
			if count > 0 {
				matchedTerms++
				// Diminishing returns on term frequency (BM25-like saturation)
				score += float32(count) / float32(count+2)
			}

			// Boost if term matches file path
			if lowerFilePath != "" && strings.Contains(lowerFilePath, term) {
				score += 1.5
			}
		}

		// Boost if high fraction of query terms found
		if len(terms) > 0 && matchedTerms > 0 {
			coverage := float32(matchedTerms) / float32(len(terms))
			score += coverage * 3.0
		}

		if score > 0 {
			copied := *cand
			copied.Score = score
			scored = append(scored, scoredMatch{
				match: &copied,
				score: score,
			})
		}
	}

	// Sort descending by lexical score
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	results := make([]*model.ChunkMatch, len(scored))
	for i, s := range scored {
		results[i] = s.match
	}

	return results
}

// termRegex splits queries into alphanumeric terms and code identifiers.
var termRegex = regexp.MustCompile(`[a-zA-Z0-9_]+`)

func extractTerms(text string) []string {
	matches := termRegex.FindAllString(text, -1)
	if len(matches) == 0 {
		return strings.Fields(text)
	}
	return matches
}

// matchesAnyFileFilter checks if a filePath matches any of the filter glob patterns.
func matchesAnyFileFilter(filePath string, filters []string) bool {
	cleanPath := filepath.Clean(filePath)

	for _, filter := range filters {
		cleanFilter := filepath.Clean(filter)
		// Exact match
		if cleanPath == cleanFilter {
			return true
		}

		// Direct glob match (e.g. *.go)
		if matched, _ := filepath.Match(filter, cleanPath); matched {
			return true
		}
		if matched, _ := filepath.Match(filter, filepath.Base(cleanPath)); matched {
			return true
		}

		// Directory prefix match (e.g. "internal/auth/*" or "internal/auth")
		prefix := strings.TrimSuffix(cleanFilter, "/*")
		prefix = strings.TrimSuffix(prefix, "/**")
		prefix = strings.TrimSuffix(prefix, "*")
		if prefix != "" && (strings.HasPrefix(cleanPath, prefix) || strings.HasPrefix(cleanPath, strings.TrimPrefix(prefix, "/"))) {
			return true
		}
	}

	return false
}
