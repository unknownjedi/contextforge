package ingest

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/model"
)

// DedupCache tracks existing chunk content hashes and their embeddings within a project.
type DedupCache interface {
	GetEmbedding(ctx context.Context, projectID uuid.UUID, contentHash string) ([]float32, bool)
	PutEmbedding(ctx context.Context, projectID uuid.UUID, contentHash string, embedding []float32)
}

// MemoryDedupCache is an in-memory thread-safe implementation of DedupCache.
type MemoryDedupCache struct {
	mu    sync.RWMutex
	cache map[string][]float32 // key: projectID:contentHash
}

// NewMemoryDedupCache initializes an empty in-memory deduplication cache.
func NewMemoryDedupCache() *MemoryDedupCache {
	return &MemoryDedupCache{
		cache: make(map[string][]float32),
	}
}

func makeKey(projectID uuid.UUID, hash string) string {
	return projectID.String() + ":" + hash
}

func (m *MemoryDedupCache) GetEmbedding(ctx context.Context, projectID uuid.UUID, contentHash string) ([]float32, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	emb, found := m.cache[makeKey(projectID, contentHash)]
	if !found || len(emb) == 0 {
		return nil, false
	}
	// Return a copy
	res := make([]float32, len(emb))
	copy(res, emb)
	return res, true
}

func (m *MemoryDedupCache) PutEmbedding(ctx context.Context, projectID uuid.UUID, contentHash string, embedding []float32) {
	if len(embedding) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	copied := make([]float32, len(embedding))
	copy(copied, embedding)
	m.cache[makeKey(projectID, contentHash)] = copied
}

// DedupResult holds the partitioned chunks after duplicate analysis.
type DedupResult struct {
	ChunksToEmbed   []*model.DocumentChunk
	CachedChunks    []*model.DocumentChunk
	EmbeddingsSaved int
}

// FilterDuplicateChunks inspects a list of document chunks and reuses embeddings for identical content hashes.
func FilterDuplicateChunks(ctx context.Context, cache DedupCache, projectID uuid.UUID, chunks []*model.DocumentChunk) *DedupResult {
	res := &DedupResult{}

	for _, chunk := range chunks {
		if emb, found := cache.GetEmbedding(ctx, projectID, chunk.ContentHash); found {
			chunk.Embedding = emb
			res.CachedChunks = append(res.CachedChunks, chunk)
			res.EmbeddingsSaved++
		} else {
			res.ChunksToEmbed = append(res.ChunksToEmbed, chunk)
		}
	}

	return res
}
