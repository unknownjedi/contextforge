-- 000002_add_hnsw_indexes.up.sql
-- Create HNSW vector similarity indexes for supported embedding dimensions.
-- Note: PostgreSQL pgvector requires explicit dimensions on HNSW index expressions.
-- Partial indexes allow supporting multiple embedding dimensions (e.g. 768 for Ollama/Gemini, 1536 for OpenAI).

CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw_768 
ON document_chunks 
USING hnsw ((embedding::vector(768)) vector_cosine_ops) 
WITH (m = 16, ef_construction = 64)
WHERE (vector_dims(embedding) = 768);

CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw_1536 
ON document_chunks 
USING hnsw ((embedding::vector(1536)) vector_cosine_ops) 
WITH (m = 16, ef_construction = 64)
WHERE (vector_dims(embedding) = 1536);

-- Composite index for rapid tenant-scoped document chunk operations
CREATE INDEX IF NOT EXISTS idx_chunks_project_doc ON document_chunks(project_id, document_id);

