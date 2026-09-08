-- 000002_add_hnsw_indexes.up.sql
-- Create HNSW vector similarity index for cosine distance
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw 
ON document_chunks 
USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

-- Composite index for rapid tenant-scoped document chunk operations
CREATE INDEX IF NOT EXISTS idx_chunks_project_doc ON document_chunks(project_id, document_id);
