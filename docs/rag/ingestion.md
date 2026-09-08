# ContextForge Ingestion Pipeline

## Ingestion Architecture

```
[External Source] (GitHub / Web / Upload)
       │
       ▼
 [SourceConnector.Discover] ──► DiscoveredItems (ExternalID, Hash, URL)
       │
       ▼
 [Content Hashing] ───────────► Check SHA-256 in DB (Skip unchanged items)
       │
       ▼
 [SourceConnector.Fetch] ──────► RawDocument (Content, Language)
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

## Deduplication Strategy

1. **Repository-Level**: A canonical `GitHubRepository` row is shared across projects. Distributed locks (`lock:repo:{id}`) ensure concurrent syncs for the same repo share cloning operations.
2. **File-Level**: Each file's normalized content is hashed using SHA-256 (`content_hash`). Unchanged files skip re-chunking and re-embedding.
3. **Incremental GitHub Sync**: Tracks `last_synced_commit_sha` and calls GitHub Compare API to isolate added, modified, and deleted files.
