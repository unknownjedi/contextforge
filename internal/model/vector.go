package model

import (
	"time"

	"github.com/google/uuid"
)

// DocumentChunk represents a semantic chunk of code or documentation with its vector embedding.
type DocumentChunk struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	DocumentID  uuid.UUID `json:"document_id"`
	ChunkIndex  int       `json:"chunk_index"`
	StartLine   int       `json:"start_line"`
	EndLine     int       `json:"end_line"`
	Content     string    `json:"content"`
	ContentHash string    `json:"content_hash"`
	TokenCount  int       `json:"token_count"`
	Embedding   []float32 `json:"embedding,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ChunkMatch represents a search result from a similarity or hybrid query.
type ChunkMatch struct {
	Chunk      *DocumentChunk `json:"chunk"`
	Similarity float32        `json:"similarity"`
	Score      float32        `json:"score"` // Hybrid or RRF combined score
	SourceID   uuid.UUID      `json:"source_id,omitempty"`
	FilePath   string         `json:"file_path,omitempty"`
	Language   string         `json:"language,omitempty"`
}

// VectorSearchParams encapsulates parameters for vector similarity searches.
type VectorSearchParams struct {
	ProjectID      uuid.UUID `json:"project_id"`
	QueryEmbedding []float32 `json:"query_embedding"`
	TopK           int       `json:"top_k"`
	SimilarityMin  float32   `json:"similarity_min"`
	SourceFilters  []string  `json:"source_filters,omitempty"`
	FileFilters    []string  `json:"file_filters,omitempty"`
	HnswEfSearch   int       `json:"hnsw_ef_search,omitempty"`
}
