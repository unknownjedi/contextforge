package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/your-org/contextforge/internal/model"
)

// PgVectorRepository implements VectorRepository using native PostgreSQL and pgvector.
// All queries strictly enforce project-level data isolation via WHERE project_id = $1.
type PgVectorRepository struct {
	db *sql.DB
}

// NewPgVectorRepository initializes a pgvector repository backed by a connection pool.
func NewPgVectorRepository(db *sql.DB) *PgVectorRepository {
	return &PgVectorRepository{db: db}
}

// FormatVector converts a float32 slice into PostgreSQL pgvector text format: [0.1,0.2,...]
func FormatVector(vec []float32) string {
	if len(vec) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(fmt.Sprintf("%g", v))
	}
	b.WriteByte(']')
	return b.String()
}

// UpsertChunks inserts or updates document chunks along with their pgvector embeddings.
func (r *PgVectorRepository) UpsertChunks(ctx context.Context, chunks []*model.DocumentChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning upsert transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO document_chunks (
			id, project_id, document_id, chunk_index, start_line, end_line,
			content, content_hash, token_count, embedding
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10::vector
		)
		ON CONFLICT (document_id, chunk_index) DO UPDATE SET
			start_line = EXCLUDED.start_line,
			end_line = EXCLUDED.end_line,
			content = EXCLUDED.content,
			content_hash = EXCLUDED.content_hash,
			token_count = EXCLUDED.token_count,
			embedding = EXCLUDED.embedding
	`)
	if err != nil {
		return fmt.Errorf("preparing chunk upsert statement: %w", err)
	}
	defer stmt.Close()

	for _, c := range chunks {
		if c.ID == uuid.Nil {
			c.ID = uuid.New()
		}
		if c.ProjectID == uuid.Nil {
			return fmt.Errorf("cannot upsert chunk index %d with nil project ID", c.ChunkIndex)
		}
		if c.DocumentID == uuid.Nil {
			return fmt.Errorf("cannot upsert chunk index %d with nil document ID", c.ChunkIndex)
		}
		if len(c.Embedding) == 0 {
			return fmt.Errorf("cannot upsert chunk index %d for doc %s with empty embedding", c.ChunkIndex, c.DocumentID)
		}
		vecStr := FormatVector(c.Embedding)
		_, err := stmt.ExecContext(ctx,
			c.ID,
			c.ProjectID,
			c.DocumentID,
			c.ChunkIndex,
			c.StartLine,
			c.EndLine,
			c.Content,
			c.ContentHash,
			c.TokenCount,
			vecStr,
		)
		if err != nil {
			return fmt.Errorf("inserting chunk index %d for doc %s: %w", c.ChunkIndex, c.DocumentID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing chunk upsert: %w", err)
	}
	return nil
}

// SearchSimilar executes an HNSW-indexed cosine distance similarity search strictly filtered by project_id.
func (r *PgVectorRepository) SearchSimilar(ctx context.Context, params model.VectorSearchParams) ([]*model.ChunkMatch, error) {
	if params.ProjectID == uuid.Nil {
		return nil, fmt.Errorf("project ID is required for vector search")
	}
	if len(params.QueryEmbedding) == 0 {
		return nil, fmt.Errorf("query embedding cannot be empty")
	}

	topK := params.TopK
	if topK <= 0 {
		topK = 5
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("beginning search tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Configure local HNSW search parameter if specified
	if params.HnswEfSearch > 0 {
		_, _ = tx.ExecContext(ctx, fmt.Sprintf("SET LOCAL hnsw.ef_search = %d;", params.HnswEfSearch))
	}

	vecStr := FormatVector(params.QueryEmbedding)

	query := `
		SELECT 
			c.id, c.project_id, c.document_id, c.chunk_index, c.start_line, c.end_line,
			c.content, c.content_hash, c.token_count, c.created_at,
			d.source_id, d.file_path, d.language,
			1 - (c.embedding <=> $2::vector) AS similarity
		FROM document_chunks c
		JOIN documents d ON c.document_id = d.id
		WHERE c.project_id = $1
		ORDER BY c.embedding <=> $2::vector ASC
		LIMIT $3
	`

	rows, err := tx.QueryContext(ctx, query, params.ProjectID, vecStr, topK)
	if err != nil {
		return nil, fmt.Errorf("executing similarity query: %w", err)
	}
	defer rows.Close()

	var matches []*model.ChunkMatch
	for rows.Next() {
		var (
			c          model.DocumentChunk
			sourceID   uuid.UUID
			filePath   string
			language   string
			similarity float32
		)

		err := rows.Scan(
			&c.ID,
			&c.ProjectID,
			&c.DocumentID,
			&c.ChunkIndex,
			&c.StartLine,
			&c.EndLine,
			&c.Content,
			&c.ContentHash,
			&c.TokenCount,
			&c.CreatedAt,
			&sourceID,
			&filePath,
			&language,
			&similarity,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning chunk row: %w", err)
		}

		if params.SimilarityMin > 0 && similarity < params.SimilarityMin {
			continue
		}

		matches = append(matches, &model.ChunkMatch{
			Chunk:      &c,
			Similarity: similarity,
			Score:      similarity,
			SourceID:   sourceID,
			FilePath:   filePath,
			Language:   language,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating chunk rows: %w", err)
	}

	return matches, nil
}

// DeleteChunksByDocumentID removes all chunks for a document within a project.
func (r *PgVectorRepository) DeleteChunksByDocumentID(ctx context.Context, projectID, documentID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM document_chunks 
		WHERE project_id = $1 AND document_id = $2
	`, projectID, documentID)
	if err != nil {
		return fmt.Errorf("deleting chunks by document: %w", err)
	}
	return nil
}

// DeleteChunksByProjectID removes all chunks for a project.
func (r *PgVectorRepository) DeleteChunksByProjectID(ctx context.Context, projectID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM document_chunks 
		WHERE project_id = $1
	`, projectID)
	if err != nil {
		return fmt.Errorf("deleting chunks by project: %w", err)
	}
	return nil
}

// CountChunksByProjectID counts chunks for a project.
func (r *PgVectorRepository) CountChunksByProjectID(ctx context.Context, projectID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM document_chunks 
		WHERE project_id = $1
	`, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting chunks: %w", err)
	}
	return count, nil
}
