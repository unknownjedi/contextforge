# ContextForge Architecture Overview

ContextForge is an open-source, project-scoped RAG (Retrieval-Augmented Generation) knowledge platform. It turns GitHub repositories, external relational databases, pull requests, issues, web URLs, and documents into searchable, citation-grounded knowledge bases.

## Core Architectural Invariants

1. **Hard Project-Level Isolation**:
   - Every piece of indexed data (`document`, `document_chunk`, `ingestion_job`, `conversation`) belongs to a `project_id`.
   - Every vector similarity query against pgvector executes `WHERE project_id = $project_id` inside the primary SQL scan.
   - Post-query in-memory filtering is strictly forbidden.
   - Cross-project data leakage is architecturally impossible.

2. **Decoupled Asynchronous Ingestion**:
   - Long-running indexing jobs run asynchronously via background workers (`cmd/worker`) powered by **Asynq** and **Redis**.
   - HTTP API handlers enqueue tasks and immediately return `202 Accepted` or `201 Created` with a job tracking ID.

3. **Multi-Engine Relational Knowledge Connectors & Normalization**:
   - External databases (PostgreSQL, CockroachDB, MySQL, MariaDB, SQLite, SQL Server) are first-class knowledge sources.
   - Schemas, constraints, foreign key topologies, and sample records are normalized into deterministic SQL/Markdown knowledge documents with precise line anchors.
   - Database credentials are encrypted at rest using AES-256-GCM, targets are validated for SSRF protection, and sensitive columns are automatically excluded.

4. **Pluggable AI & Subscription Bridges**:
   - Pluggable `Embedder` interface supporting zero-cost local models (Ollama / in-process) and cloud BYOK (OpenAI, Gemini).
   - Pluggable `Generator` interface supporting cloud BYOK (OpenAI, Anthropic, Gemini) and native local CLI bridges (`opencode` with OpenCode Go subscription, `claudecode`, `gemini cli`, `codex`).

5. **Ent ORM with Encapsulated pgvector**:
   - Entity schemas, graph traversals, and transactions are managed via **Ent** (`entgo.io/ent`).
   - pgvector similarity search (`<=>`) and vector index queries are strictly encapsulated behind a dedicated `VectorRepository` interface. Higher-level services never call raw pgvector SQL directly.

6. **API-First Design**:
   - The Next.js web application is a pure client of the Go REST API.
   - All API endpoints are defined in `docs/api/openapi.yaml` and prefixed with `/api/v1`.
