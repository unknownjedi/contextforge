# ADR 0009: Strict Encapsulation of pgvector Behind VectorRepository Interface

## Status
Accepted

## Date
2026-09-08

## Context
While Ent handles standard relational entities and edges with high type-safety, specialized vector operations in `pgvector` rely on custom PostgreSQL operators (`<=>` for cosine distance, `<->` for Euclidean L2, `<#>` for negative inner product) and HNSW index scan hints (`SET LOCAL hnsw.ef_search = X`).

Allowing raw vector SQL or PostgreSQL-specific distance operators to be written directly in API controllers or business logic services introduces severe risks:
1. Accidental omission of the mandatory `WHERE project_id = $1` tenant filter, causing cross-tenant data leaks.
2. Tight coupling of core domain services to PostgreSQL dialect quirks.
3. Inability to unit-test RAG retrieval logic without a live PostgreSQL instance running the `vector` extension.

## Decision
We mandate that all `pgvector` interactions must be strictly encapsulated behind a domain-level **`VectorRepository`** interface. No handler, service, or background worker is permitted to issue raw vector queries directly.

### VectorRepository Interface Definition
```go
package repository

import (
	"context"
	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/model"
)

type VectorSearchParams struct {
	ProjectID      uuid.UUID
	QueryEmbedding []float32
	TopK           int
	SimilarityMin  float32
	SourceFilters  []string
	FileFilters    []string
	HnswEfSearch   int
}

type VectorRepository interface {
	UpsertChunks(ctx context.Context, chunks []*model.DocumentChunk) error
	SearchSimilar(ctx context.Context, params VectorSearchParams) ([]*model.ChunkMatch, error)
	DeleteChunksByDocumentID(ctx context.Context, projectID, documentID uuid.UUID) error
	DeleteChunksByProjectID(ctx context.Context, projectID uuid.UUID) error
	CountChunksByProjectID(ctx context.Context, projectID uuid.UUID) (int64, error)
}
```

### Implementation Details
- The concrete implementation (`PgVectorRepository`) uses `pgxpool` for direct, high-performance parameterized SQL execution.
- All query queries statically append `WHERE project_id = $1` as the first filtering predicate, enforcing project data isolation at the repository boundary.
- Search queries execute within a local transaction to safely configure `SET LOCAL hnsw.ef_search = X` without poisoning pool-wide connection settings.

## Consequences

### Positive
- Strict compile-time and architectural boundary: domain services remain completely oblivious to vector database storage mechanics.
- In-memory mock implementations (`MockVectorRepository`) allow blazing fast unit tests for RAG retrieval and citation ranking without database dependencies.
- Enforced project-level isolation: impossible to execute a similarity search without providing a verified `projectID`.
- Future-proofing: if another vector engine is ever needed for specialized edge deployments, only the repository implementation changes.

### Negative
- Requires maintaining both Ent ORM for relational models and a specialized `PgVectorRepository` for chunk embeddings.
- Data synchronization between relational chunks and vector representations must be managed atomically within repository operations.

## References
- SPEC.md: Section 22 (Vector Database Strategy) & Section 24 (Repository Abstraction)
- ADR 0002: PostgreSQL with pgvector for Hybrid Relational & Vector Storage
- ADR 0005: Multi-Tenant Project Data Isolation
