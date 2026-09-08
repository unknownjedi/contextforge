# ContextForge Ingestion Pipeline

## Ingestion Architecture

```
[External Source] (GitHub / External Databases / Web / Upload)
       │
       ├── GitHub: [SourceConnector.Discover] ──► DiscoveredItems (ExternalID, Hash, URL)
       │
       └── Database: [Connector.IntrospectCatalog] ──► Catalog Metadata (Tables, Columns, PKs, FKs, Indexes)
                                 │
                                 ▼
                     [KnowledgeNormalizer] ──► Normalized SQL DDL & Row Tables
       │                         │
       └─────────────────────────┘
                   │
                   ▼
  [Content Hashing] ───────────► Check SHA-256 in DB (Skip unchanged items)
       │
       ▼
  [Text/Code Chunker] ─────────► Chunks (Text, Metadata: start_line, end_line)
       │
       ▼
  [Batch Embedder] ────────────► Vectors (batch size up to 100-500)
       │
       ▼
  [Atomic DB Transaction] ─────► Save Document via Ent + Save Chunks via VectorRepository
```

## Deduplication & Incremental Strategy

1. **Repository-Level**: A canonical `GitHubRepository` row is shared across projects. Distributed locks (`lock:repo:{id}`) ensure concurrent syncs for the same repo share cloning operations.
2. **File-Level**: Each file's normalized content is hashed using SHA-256 (`content_hash`). Unchanged files skip re-chunking and re-embedding.
3. **Incremental GitHub Sync**: Tracks `last_synced_commit_sha` and calls GitHub Compare API to isolate added, modified, and deleted files.
4. **Database Table Diffing**: The `KnowledgeNormalizer` computes SHA-256 hashes for each table DDL document (`schema/{schema}/{table}.sql`). When re-syncing:
   - Unmodified tables (`content_hash` matches) bypass chunking and embedding.
   - Modified tables have obsolete vector chunks purged via `DeleteChunksByDocumentID` before newly embedded chunks are upserted.
   - Deleted tables have their document records and associated vector chunks cascade-deleted.

