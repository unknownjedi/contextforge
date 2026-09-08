package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/model"
)

// VectorRepository defines the contract for vector database interactions,
// strictly isolating pgvector operations from domain services.
type VectorRepository interface {
	// UpsertChunks inserts or updates document chunks and their high-dimensional vector embeddings.
	UpsertChunks(ctx context.Context, chunks []*model.DocumentChunk) error

	// SearchSimilar executes a nearest-neighbor similarity search scoped strictly to a project.
	SearchSimilar(ctx context.Context, params model.VectorSearchParams) ([]*model.ChunkMatch, error)

	// DeleteChunksByDocumentID removes all chunks belonging to a document within a project.
	DeleteChunksByDocumentID(ctx context.Context, projectID, documentID uuid.UUID) error

	// DeleteChunksByProjectID removes all chunks belonging to a project.
	DeleteChunksByProjectID(ctx context.Context, projectID uuid.UUID) error

	// CountChunksByProjectID returns the total count of chunks for a project.
	CountChunksByProjectID(ctx context.Context, projectID uuid.UUID) (int64, error)
}
