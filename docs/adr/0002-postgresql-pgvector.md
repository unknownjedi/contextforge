# ADR 0002: PostgreSQL with pgvector for Hybrid Relational & Vector Storage

## Status
Accepted

## Date
2026-09-08

## Context
ContextForge requires storing both highly structured relational data (users, projects, credentials, repositories, files, ingestion jobs, audit logs) and high-dimensional semantic vector embeddings (768 to 3072 dimensions) with sub-second nearest-neighbor search.

We evaluated three architectural topologies:
1. **Unified Relational + Vector (PostgreSQL 16 + pgvector)**: Single database engine handling ACID transactions, foreign keys, row-level filtering, and HNSW/IVFFlat vector indexing.
2. **Dual-Store Architecture (PostgreSQL + Specialized Vector DB like Pinecone, Qdrant, or Weaviate)**: Relational metadata stored in Postgres, vector embeddings stored in a dedicated vector service.
3. **Pure Document/Vector DB (Weaviate or Milvus as primary)**: Storing all relational data and vectors inside an AI-native database.

## Decision
We chose **PostgreSQL 16 with the `pgvector` extension** as the unified storage engine for both relational state and semantic vector embeddings.

Key architectural drivers:
- **Transactional Consistency (ACID)**: Document deletion, project deletion, or updates atomically cascade to corresponding chunks and embeddings without distributed transaction complexity or dual-write inconsistencies.
- **Pre-Filtering Performance**: Combining relational predicates (`project_id = X AND source_type = 'github'`) directly with vector distance queries (`ORDER BY embedding <=> query_vector LIMIT K`) eliminates post-filtering overhead and synchronization lag.
- **Operational Simplicity**: Self-hosted developers and production operators only need to run, backup, monitor, and scale a single stateful storage system.
- **HNSW Indexing**: `pgvector` provides state-of-the-art Hierarchical Navigable Small World (HNSW) graphs with cosine (`<=>`), L2 (`<->`), and inner product (`<#>`) operators.

## Consequences

### Positive
- Single database backup (`pg_dump`), single connection pool, and unified disaster recovery.
- Elimination of distributed sync lag between vector store and relational metadata.
- Native SQL support for hybrid search (PostgreSQL `tsvector`/BM25 combined with `pgvector` cosine similarity).
- Zero proprietary cloud lock-in: works on AWS RDS, Supabase, Cloud SQL, Azure Database, or plain Docker.

### Negative
- PostgreSQL connection memory overhead must be managed with connection pooling (`pgxpool`).
- Extremely large vector datasets (> 50M embeddings) require dedicated PostgreSQL memory tuning (`shared_buffers`, `maintenance_work_mem`) to hold HNSW graphs in RAM.

## Alternatives Considered
- **Pinecone / Weaviate / Qdrant**: Rejected due to operational complexity of dual-store syncing, distributed transaction edge-cases, cost, and lack of full self-hosted offline capability for local development.

## References
- SPEC.md: Section 22 (Vector Database Strategy) & Section 23 (Indexing Strategy)
- [pgvector GitHub Repository](https://github.com/pgvector/pgvector)
