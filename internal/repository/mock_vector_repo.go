package repository

import (
	"context"
	"math"
	"sort"
	"sync"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/model"
)

// MockVectorRepository provides a thread-safe, in-memory implementation of VectorRepository
// with real cosine similarity calculation for fast unit testing.
type MockVectorRepository struct {
	mu        sync.RWMutex
	chunks    map[uuid.UUID]*model.DocumentChunk
	filePaths map[uuid.UUID]string
	sourceIDs map[uuid.UUID]uuid.UUID
}

// NewMockVectorRepository initializes an empty mock vector repository.
func NewMockVectorRepository() *MockVectorRepository {
	return &MockVectorRepository{
		chunks:    make(map[uuid.UUID]*model.DocumentChunk),
		filePaths: make(map[uuid.UUID]string),
		sourceIDs: make(map[uuid.UUID]uuid.UUID),
	}
}

// SetChunkMetadata associates file path and source ID metadata with a chunk in mock repository.
func (m *MockVectorRepository) SetChunkMetadata(chunkID uuid.UUID, filePath string, sourceID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.filePaths == nil {
		m.filePaths = make(map[uuid.UUID]string)
	}
	if m.sourceIDs == nil {
		m.sourceIDs = make(map[uuid.UUID]uuid.UUID)
	}
	m.filePaths[chunkID] = filePath
	m.sourceIDs[chunkID] = sourceID
}

// UpsertChunks stores or updates chunks in memory.
func (m *MockVectorRepository) UpsertChunks(ctx context.Context, chunks []*model.DocumentChunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, chunk := range chunks {
		if chunk.ID == uuid.Nil {
			chunk.ID = uuid.New()
		}
		// Deep copy chunk to avoid caller mutation races
		copied := *chunk
		if chunk.Embedding != nil {
			copied.Embedding = make([]float32, len(chunk.Embedding))
			copy(copied.Embedding, chunk.Embedding)
		}
		m.chunks[chunk.ID] = &copied
	}
	return nil
}

// SearchSimilar executes an in-memory cosine similarity search scoped strictly to params.ProjectID.
func (m *MockVectorRepository) SearchSimilar(ctx context.Context, params model.VectorSearchParams) ([]*model.ChunkMatch, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type matchWithSim struct {
		chunk *model.DocumentChunk
		sim   float32
	}

	var candidateMatches []matchWithSim

	for _, chunk := range m.chunks {
		// Strict project-level isolation check
		if chunk.ProjectID != params.ProjectID {
			continue
		}

		if len(chunk.Embedding) == 0 || len(params.QueryEmbedding) == 0 {
			continue
		}

		sim := cosineSimilarity(params.QueryEmbedding, chunk.Embedding)
		if params.SimilarityMin <= 0 || sim >= params.SimilarityMin {
			candidateMatches = append(candidateMatches, matchWithSim{
				chunk: chunk,
				sim:   sim,
			})
		}
	}

	// Sort descending by similarity
	sort.Slice(candidateMatches, func(i, j int) bool {
		return candidateMatches[i].sim > candidateMatches[j].sim
	})

	limit := params.TopK
	if limit <= 0 || limit > len(candidateMatches) {
		limit = len(candidateMatches)
	}

	results := make([]*model.ChunkMatch, 0, limit)
	for i := 0; i < limit; i++ {
		match := candidateMatches[i]
		cm := &model.ChunkMatch{
			Chunk:      match.chunk,
			Similarity: match.sim,
			Score:      match.sim,
		}
		if m.filePaths != nil {
			if fp, ok := m.filePaths[match.chunk.ID]; ok {
				cm.FilePath = fp
			}
		}
		if m.sourceIDs != nil {
			if sid, ok := m.sourceIDs[match.chunk.ID]; ok {
				cm.SourceID = sid
			}
		}
		results = append(results, cm)
	}

	return results, nil
}

// DeleteChunksByDocumentID removes all chunks for a document within a project.
func (m *MockVectorRepository) DeleteChunksByDocumentID(ctx context.Context, projectID, documentID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, chunk := range m.chunks {
		if chunk.ProjectID == projectID && chunk.DocumentID == documentID {
			delete(m.chunks, id)
		}
	}
	return nil
}

// DeleteChunksByProjectID deletes all chunks belonging to a project.
func (m *MockVectorRepository) DeleteChunksByProjectID(ctx context.Context, projectID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, chunk := range m.chunks {
		if chunk.ProjectID == projectID {
			delete(m.chunks, id)
		}
	}
	return nil
}

// CountChunksByProjectID returns chunk count for a project.
func (m *MockVectorRepository) CountChunksByProjectID(ctx context.Context, projectID uuid.UUID) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var count int64
	for _, chunk := range m.chunks {
		if chunk.ProjectID == projectID {
			count++
		}
	}
	return count, nil
}

// GetChunksByDocumentID returns all chunks for a specific document within a project.
func (m *MockVectorRepository) GetChunksByDocumentID(ctx context.Context, projectID, documentID uuid.UUID) ([]*model.DocumentChunk, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*model.DocumentChunk
	for _, chunk := range m.chunks {
		if chunk.ProjectID == projectID && chunk.DocumentID == documentID {
			copied := *chunk
			if chunk.Embedding != nil {
				copied.Embedding = make([]float32, len(chunk.Embedding))
				copy(copied.Embedding, chunk.Embedding)
			}
			results = append(results, &copied)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ChunkIndex < results[j].ChunkIndex
	})

	return results, nil
}

// Helper: computes cosine similarity between two float32 vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := 0; i < len(a); i++ {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
