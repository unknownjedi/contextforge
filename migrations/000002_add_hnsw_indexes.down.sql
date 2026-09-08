-- 000002_add_hnsw_indexes.down.sql
DROP INDEX IF EXISTS idx_chunks_project_doc;
DROP INDEX IF EXISTS idx_chunks_embedding_hnsw_1536;
DROP INDEX IF EXISTS idx_chunks_embedding_hnsw_768;
DROP INDEX IF EXISTS idx_chunks_embedding_hnsw;

