# ContextForge Implementation Plan & Task Backlog

This document is the single source of truth for the autonomous development and verification of ContextForge.
All tasks are strictly sequential by milestone, track dependencies, specify exact impacted files, acceptance criteria, and explicit terminal verification commands.

---

## Task Progress Summary

| Milestone | Title | Tasks | Status |
| :--- | :--- | :--- | :--- |
| **M1** | Project Foundation & Tooling | CF-001 – CF-004 | **DONE** (4/4) |
| **M2** | Database Layer & Ent Migrations | CF-005 – CF-009 | **DONE** (5/5) |
| **M3** | Cryptography, Auth & GitHub Integration | CF-010 – CF-014 | **DONE** (5/5) |
| **M4** | Vector Repository & Provider Abstractions | CF-015 – CF-020 | **DONE** (6/6) |
| **M5** | Ingestion Pipeline & Background Workers | CF-021 – CF-026 | **DONE** (6/6) |
| **M6** | RAG Engine, Retrieval & Chat Streaming | CF-027 – CF-030 | **DONE** (4/4) |
| **M7** | Web Frontend & Developer UI | CF-031 – CF-034 | **DONE** (4/4) |
| **M8** | Security Hardening, E2E & Production Readiness | CF-035 – CF-038 | **DONE** (4/4) |
| **M9** | External Relational Database Knowledge Sources | CF-039 – CF-044 | **DONE** (6/6) |


---

## Milestone 1: Project Foundation & Tooling

### `CF-001`: Go Module & Dependency Baseline
- **Milestone**: M1
- **Dependencies**: None
- **Impacted Files**:
  - `go.mod`
  - `go.sum`
- **Description**: Initialize Go module `github.com/your-org/contextforge` with Go 1.23 toolchain. Install foundational dependencies: `gin-gonic/gin`, `entgo.io/ent`, `google/uuid`, `hibiken/asynq`, `jackc/pgx/v5`, `redis/go-redis/v9`, `uber-go/zap`, `spf13/viper`, `stretchr/testify`.
- **Acceptance Criteria**:
  - `go.mod` is cleanly initialized.
  - Dependencies are fetched and pinned in `go.sum`.
  - `go vet ./...` succeeds with zero warnings.
- **Verification Command**:
  ```bash
  go vet ./... && go mod verify
  ```
- **Status**: `DONE`

---

### `CF-002`: Configuration & Environment Loader
- **Milestone**: M1
- **Dependencies**: CF-001
- **Impacted Files**:
  - `internal/config/config.go`
  - `internal/config/config_test.go`
- **Description**: Build strongly-typed configuration management using Viper. Support `.env` reading, environment variable overrides with `CF_` prefix, default values for local development, and validation rules (e.g. key length checks for `CF_AUTH_TOKEN_ENCRYPTION_KEY`).
- **Acceptance Criteria**:
  - Validates presence of mandatory keys or assigns sensible dev defaults.
  - Correctly parses database URLs, Redis URLs, timeouts, and provider toggles.
  - Unit tests achieve 100% coverage of validation logic.
- **Verification Command**:
  ```bash
  go test -v ./internal/config/...
  ```
- **Status**: `DONE`

---

### `CF-003`: Structured Logging & Telemetry Initialization
- **Milestone**: M1
- **Dependencies**: CF-001, CF-002
- **Impacted Files**:
  - `internal/logger/logger.go`
  - `internal/logger/logger_test.go`
- **Description**: Implement production Zap logger with JSON production encoder and colorized development console encoder. Support request correlation IDs (`X-Correlation-ID`), contextual fields, and log level dynamic configuration.
- **Acceptance Criteria**:
  - Structured fields (`trace_id`, `project_id`, `duration_ms`) serialize correctly in JSON format.
  - Unit tests verify context injection and level filtering.
- **Verification Command**:
  ```bash
  go test -v ./internal/logger/...
  ```
- **Status**: `DONE`

---

### `CF-004`: Server Bootstrap & Health Probes (`/healthz`, `/readyz`)
- **Milestone**: M1
- **Dependencies**: CF-002, CF-003
- **Impacted Files**:
  - `cmd/api/main.go`
  - `internal/api/server.go`
  - `internal/api/handler/health.go`
  - `internal/api/handler/health_test.go`
- **Description**: Setup Gin engine with recovery, CORS, correlation ID middleware, and graceful shutdown handling (`SIGTERM`/`SIGINT`). Implement `GET /healthz` (liveness) and `GET /readyz` (readiness) with dependency checks.
- **Acceptance Criteria**:
  - API boots cleanly and responds to HTTP requests.
  - `/healthz` returns `{"status":"ok"}`.
  - Graceful shutdown drains in-flight requests within timeout.
- **Verification Command**:
  ```bash
  go test -v ./internal/api/handler/health_test.go
  ```
- **Status**: `DONE`

---

## Milestone 2: Database Layer & Ent Migrations

### `CF-005`: Ent Schema Definitions
- **Milestone**: M2
- **Dependencies**: CF-001
- **Impacted Files**:
  - `internal/ent/schema/user.go`
  - `internal/ent/schema/project.go`
  - `internal/ent/schema/source.go`
  - `internal/ent/schema/document.go`
  - `internal/ent/schema/document_chunk.go`
  - `internal/ent/schema/ingestion_job.go`
  - `internal/ent/schema/audit_log.go`
  - `internal/ent/generate.go`
- **Description**: Define declarative Ent schemas with UUID primary keys, timestamps, indexes, unique constraints, and foreign key edges. Enforce `owner_user_id` on Project, and `project_id` on all subordinate entities.
- **Acceptance Criteria**:
  - All schemas pass `go generate ./internal/ent`.
  - Compile errors and circular dependencies are eliminated.
- **Verification Command**:
  ```bash
  go generate ./internal/ent && go test ./internal/ent/...
  ```
- **Status**: `DONE`

---

### `CF-006`: Initial SQL Migrations (golang-migrate)
- **Milestone**: M2
- **Dependencies**: CF-005
- **Impacted Files**:
  - `migrations/000001_initial_schema.up.sql`
  - `migrations/000001_initial_schema.down.sql`
- **Description**: Generate initial clean DDL migrations incorporating `uuid-ossp`, `vector` extension creation, relational tables, foreign key constraints (`ON DELETE CASCADE`), and primary indexes.
- **Acceptance Criteria**:
  - `migrate -path migrations -database "${CF_DATABASE_URL}" up` runs successfully against Postgres 16.
  - Down migration rolls back cleanly without leaving orphan objects.
- **Verification Command**:
  ```bash
  make migrate-up && make migrate-down && make migrate-up
  ```
- **Status**: `DONE`

---

### `CF-007`: HNSW Vector Index Migration
- **Milestone**: M2
- **Dependencies**: CF-006
- **Impacted Files**:
  - `migrations/000002_add_hnsw_indexes.up.sql`
  - `migrations/000002_add_hnsw_indexes.down.sql`
- **Description**: Add HNSW vector indexes (`vector_cosine_ops`, `m=16`, `ef_construction=64`) on `document_chunks(embedding)` and composite index on `(project_id, document_id)`.
- **Acceptance Criteria**:
  - Vector index creation succeeds and query planner utilizes index scans for cosine distance.
- **Verification Command**:
  ```bash
  make migrate-up && docker compose exec postgres psql -U contextforge -d contextforge -c "\d document_chunks"
  ```
- **Status**: `DONE`

---

### `CF-008`: Ent Database Client & Connection Pooling
- **Milestone**: M2
- **Dependencies**: CF-005, CF-006
- **Impacted Files**:
  - `internal/database/database.go`
  - `internal/database/database_test.go`
- **Description**: Initialize Ent client backed by PostgreSQL `database/sql` driver with production connection pooling (`SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`).
- **Acceptance Criteria**:
  - Connectivity test passes against test database.
  - Connection pooling limits are validated.
- **Verification Command**:
  ```bash
  go test -v ./internal/database/...
  ```
- **Status**: `DONE`

---

### `CF-009`: Relational CRUD Repository Implementations
- **Milestone**: M2
- **Dependencies**: CF-008
- **Impacted Files**:
  - `internal/repository/project_repo.go`
  - `internal/repository/source_repo.go`
  - `internal/repository/document_repo.go`
  - `internal/repository/repo_test.go`
- **Description**: Implement repositories wrapping Ent client for Projects, Sources, Documents, and Ingestion Jobs with explicit error handling and domain mapping.
- **Acceptance Criteria**:
  - All CRUD operations tested with isolated transactional rollbacks.
- **Verification Command**:
  ```bash
  go test -v ./internal/repository/...
  ```
- **Status**: `DONE`

---

## Milestone 3: Cryptography, Auth & GitHub Integration

### `CF-010`: AES-256-GCM Cryptographic Engine
- **Milestone**: M3
- **Dependencies**: CF-002
- **Impacted Files**:
  - `internal/crypto/encrypt.go`
  - `internal/crypto/encrypt_test.go`
- **Description**: Implement authenticated encryption at rest using AES-256-GCM. Require 32-byte key, generate random 12-byte nonce per encryption, prepend nonce to ciphertext, verify MAC on decryption.
- **Acceptance Criteria**:
  - Unit tests verify roundtrip encryption/decryption, tampered ciphertext rejection, and invalid key length errors.
- **Verification Command**:
  ```bash
  go test -v ./internal/crypto/...
  ```
- **Status**: `DONE`

---

### `CF-011`: JWT & Session Management
- **Milestone**: M3
- **Dependencies**: CF-002, CF-008
- **Impacted Files**:
  - `internal/auth/jwt.go`
  - `internal/auth/middleware.go`
  - `internal/auth/jwt_test.go`
- **Description**: Implement HMAC-SHA256 signed JWT tokens with claims (`sub`, `login`, `exp`, `jti`). Create Gin authentication middleware validating tokens from `Authorization: Bearer <token>` or `cf_session` cookie.
- **Acceptance Criteria**:
  - Valid tokens inject `userID` into request context.
  - Expired, tampered, or missing tokens return RFC 7807 401 Unauthorized.
- **Verification Command**:
  ```bash
  go test -v ./internal/auth/...
  ```
- **Status**: `DONE`

---

### `CF-012`: Single-User Project Authorization Middleware
- **Milestone**: M3
- **Dependencies**: CF-009, CF-011
- **Impacted Files**:
  - `internal/api/middleware/tenant.go`
  - `internal/api/middleware/tenant_test.go`
- **Description**: Create middleware verifying that the authenticated user matches `project.owner_user_id`. Return 404/403 if project does not exist or belongs to another user.
- **Acceptance Criteria**:
  - Rejects cross-project requests from unauthorized users.
- **Verification Command**:
  ```bash
  go test -v ./internal/api/middleware/...
  ```
- **Status**: `DONE`

---

### `CF-013`: GitHub OAuth & PAT Fallback Handler
- **Milestone**: M3
- **Dependencies**: CF-010, CF-011
- **Impacted Files**:
  - `internal/api/handler/auth.go`
  - `internal/service/github_auth.go`
  - `internal/api/handler/auth_test.go`
- **Description**: Implement `/auth/github/login`, `/auth/github/callback`, and `/auth/pat`. Store encrypted tokens at rest.
- **Acceptance Criteria**:
  - Successful OAuth exchanges token, upserts User, encrypts token, and issues session cookie.
  - PAT fallback validates token against `api.github.com/user` and provisions local dev user.
- **Verification Command**:
  ```bash
  go test -v ./internal/api/handler/auth_test.go
  ```
- **Status**: `DONE`

---

### `CF-014`: GitHub Webhook Signature Verification & Ingress
- **Milestone**: M3
- **Dependencies**: CF-002, CF-009
- **Impacted Files**:
  - `internal/api/handler/webhook.go`
  - `internal/api/handler/webhook_test.go`
- **Description**: Implement `POST /github/webhooks` with HMAC-SHA256 signature verification (`X-Hub-Signature-256`). Parse `push` events and trigger synchronization jobs.
- **Acceptance Criteria**:
  - Rejects unverified signatures with 401.
  - Queues sync task for matching repository sources on verified push events.
- **Verification Command**:
  ```bash
  go test -v ./internal/api/handler/webhook_test.go
  ```
- **Status**: `DONE`

---

## Milestone 4: Vector Repository & Provider Abstractions

### `CF-015`: VectorRepository Interface & Mock
- **Milestone**: M4
- **Dependencies**: CF-001
- **Impacted Files**:
  - `internal/repository/vector_repo.go`
  - `internal/repository/mock_vector_repo.go`
  - `internal/model/vector.go`
- **Description**: Define strict `VectorRepository` interface (`UpsertChunks`, `SearchSimilar`, `DeleteChunksByDocumentID`, `DeleteChunksByProjectID`). Provide memory-backed mock for testing.
- **Acceptance Criteria**:
  - Interface decouples vector storage mechanics from domain services.
  - Mock passes all repository unit tests.
- **Verification Command**:
  ```bash
  go test -v ./internal/repository/vector_repo_test.go
  ```
- **Status**: `DONE`

---

### `CF-016`: PostgreSQL pgvector Repository Implementation
- **Milestone**: M4
- **Dependencies**: CF-007, CF-015
- **Impacted Files**:
  - `internal/repository/pgvector_repo.go`
  - `internal/repository/pgvector_repo_test.go`
- **Description**: Implement `PgVectorRepository` using `jackc/pgx/v5`. Enforce `WHERE project_id = $1` on every query. Execute similarity searches with cosine distance `<=>` and `SET LOCAL hnsw.ef_search = X`.
- **Acceptance Criteria**:
  - Integration tests verify chunk insertion, cosine ranking, and tenant isolation.
- **Verification Command**:
  ```bash
  go test -v ./internal/repository/pgvector_repo_test.go
  ```
- **Status**: `DONE`

---

### `CF-017`: LLMProvider Interface & Factory
- **Milestone**: M4
- **Dependencies**: CF-002
- **Impacted Files**:
  - `internal/provider/llm.go`
  - `internal/provider/factory.go`
  - `internal/model/llm.go`
- **Description**: Define `LLMProvider` interface (`GenerateCompletion`, `StreamCompletion`). Implement factory registering OpenAI, Anthropic, Gemini, Ollama, and CLI bridges.
- **Acceptance Criteria**:
  - Clean decoupled interface with unified request/response structs and stream chunk callback.
- **Verification Command**:
  ```bash
  go test -v ./internal/provider/factory_test.go
  ```
- **Status**: `DONE`

---

### `CF-018`: Cloud BYOK Providers (OpenAI, Anthropic, Gemini)
- **Milestone**: M4
- **Dependencies**: CF-017
- **Impacted Files**:
  - `internal/provider/openai.go`
  - `internal/provider/anthropic.go`
  - `internal/provider/gemini.go`
  - `internal/provider/byok_test.go`
- **Description**: Implement cloud BYOK drivers with client-supplied API keys. Support token streaming and exponential backoff on HTTP 429.
- **Acceptance Criteria**:
  - Mock server unit tests verify payload serialization, SSE parsing, and error mapping.
- **Verification Command**:
  ```bash
  go test -v ./internal/provider/byok_test.go
  ```
- **Status**: `DONE`

---

### `CF-019`: Host CLI Bridge (`opencode`, `claude`, `gemini`)
- **Milestone**: M4
- **Dependencies**: CF-017
- **Impacted Files**:
  - `internal/provider/cli_bridge.go`
  - `internal/provider/cli_bridge_test.go`
- **Description**: Implement native host CLI execution bridge using `os/exec.CommandContext`. Support OpenCode CLI (`opencode`), Claude Code (`claude`), and Gemini CLI. Enforce timeout guards, process group cleanup, and stdout streaming.
- **Acceptance Criteria**:
  - Successfully formats prompts, executes subcommands, and handles timeouts gracefully.
- **Verification Command**:
  ```bash
  go test -v ./internal/provider/cli_bridge_test.go
  ```
- **Status**: `DONE`

---

### `CF-020`: EmbeddingProvider Interface & Ollama Driver
- **Milestone**: M4
- **Dependencies**: CF-002
- **Impacted Files**:
  - `internal/provider/embedding.go`
  - `internal/provider/embedding_ollama.go`
  - `internal/provider/embedding_openai.go`
  - `internal/provider/embedding_test.go`
- **Description**: Implement `EmbeddingProvider` interface with local Ollama driver (`nomic-embed-text`) and OpenAI BYOK driver (`text-embedding-3-small`). Support batch embedding requests.
- **Acceptance Criteria**:
  - Batching splits requests appropriately and returns normalized float32 embedding slices.
- **Verification Command**:
  ```bash
  go test -v ./internal/provider/embedding_test.go
  ```
- **Status**: `DONE`

---

## Milestone 5: Ingestion Pipeline & Background Workers

### `CF-021`: Asynq Redis Queue Setup & Worker Engine
- **Milestone**: M5
- **Dependencies**: CF-002
- **Impacted Files**:
  - `cmd/worker/main.go`
  - `internal/queue/queue.go`
  - `internal/queue/worker.go`
  - `internal/queue/tasks.go`
  - `internal/queue/queue_test.go`
- **Description**: Setup Asynq client and server. Configure `critical`, `default`, and `low` priority queues. Define task types `task:repo:sync` and `task:document:embed`.
- **Acceptance Criteria**:
  - Tasks can be enqueued with idempotency keys and processed by handler functions with retries.
- **Verification Command**:
  ```bash
  go test -v ./internal/queue/...
  ```
- **Status**: `DONE`

---

### `CF-022`: GitHub Repository Cloner & Tree Scanner
- **Milestone**: M5
- **Dependencies**: CF-010, CF-013
- **Impacted Files**:
  - `internal/ingest/scanner.go`
  - `internal/ingest/scanner_test.go`
- **Description**: Implement repository scanner using GitHub Git Trees API and fallback shallow clone. Filter out binary files, minified bundles, vendor paths, and lockfiles using `.cfignore` patterns.
- **Acceptance Criteria**:
  - Discovers source files while ignoring binary assets, `.git`, `node_modules`, and files > 1MB.
- **Verification Command**:
  ```bash
  go test -v ./internal/ingest/scanner_test.go
  ```
- **Status**: `DONE`

---

### `CF-023`: Incremental Git Synchronization & Content Hashing
- **Milestone**: M5
- **Dependencies**: CF-009, CF-022
- **Impacted Files**:
  - `internal/ingest/sync.go`
  - `internal/ingest/sync_test.go`
- **Description**: Compare Git commit trees or SHA-256 content hashes between sync cycles. Identify Added, Modified, Unchanged, and Deleted files. Only process changed files.
- **Acceptance Criteria**:
  - Unchanged files are skipped (0 re-embeddings).
  - Deleted files trigger cascading chunk deletion.
  - Modified files re-chunk atomically.
- **Verification Command**:
  ```bash
  go test -v ./internal/ingest/sync_test.go
  ```
- **Status**: `DONE`

---

### `CF-024`: Multi-Language AST & Line-Preserving Chunker
- **Milestone**: M5
- **Dependencies**: CF-001
- **Impacted Files**:
  - `internal/chunk/chunker.go`
  - `internal/chunk/tokenizer.go`
  - `internal/chunk/chunker_test.go`
- **Description**: Implement structural chunking respecting function, class, and method boundaries for Go, TypeScript/JS, Python, and Markdown. Preserve exact 1-indexed `start_line` and `end_line` coordinates. Enforce token limits (256-512 tokens) with 50-token sliding overlap.
- **Acceptance Criteria**:
  - Line numbers match source code 1:1.
  - Code syntax boundaries are preserved where possible.
- **Verification Command**:
  ```bash
  go test -v ./internal/chunk/...
  ```
- **Status**: `DONE`

---

### `CF-025`: Deduplication Pipeline & Exact-Match Hash Registry
- **Milestone**: M5
- **Dependencies**: CF-009, CF-024
- **Impacted Files**:
  - `internal/ingest/dedup.go`
  - `internal/ingest/dedup_test.go`
- **Description**: Compute SHA-256 hashes per file and chunk. Maintain project-scoped hash index to skip embedding generation for identical code snippets across repositories.
- **Acceptance Criteria**:
  - Duplicate chunk content within a project reuses existing embedding vector.
- **Verification Command**:
  ```bash
  go test -v ./internal/ingest/dedup_test.go
  ```
- **Status**: `DONE`

---

### `CF-026`: End-to-End Ingestion Worker Pipeline
- **Milestone**: M5
- **Dependencies**: CF-016, CF-020, CF-021, CF-023, CF-024, CF-025
- **Impacted Files**:
  - `internal/worker/ingest_handler.go`
  - `internal/worker/ingest_handler_test.go`
- **Description**: Assemble complete worker task handler: scan repo -> diff changes -> chunk files -> generate embeddings -> batch upsert to `VectorRepository` -> update job progress.
- **Acceptance Criteria**:
  - Processes sample repository, populates `document_chunks` table, and marks job as `completed`.
- **Verification Command**:
  ```bash
  go test -v ./internal/worker/...
  ```
- **Status**: `DONE`

---

## Milestone 6: RAG Engine, Retrieval & Chat Streaming

### `CF-027`: Hybrid Retrieval Engine & Reciprocal Rank Fusion (RRF)
- **Milestone**: M6
- **Dependencies**: CF-016, CF-020
- **Impacted Files**:
  - `internal/retrieval/retriever.go`
  - `internal/retrieval/rrf.go`
  - `internal/retrieval/retriever_test.go`
- **Description**: Implement hybrid search combining dense vector similarity (`pgvector` cosine) and sparse lexical search (PostgreSQL `tsvector` with `ts_rank_cd`). Merge results via Reciprocal Rank Fusion ($k=60$).
- **Acceptance Criteria**:
  - Retrieves exact keyword matches (function names) and conceptual matches with balanced ranking.
- **Verification Command**:
  ```bash
  go test -v ./internal/retrieval/...
  ```
- **Status**: `DONE`

---

### `CF-028`: Structured Citation Generator & Line Anchoring
- **Milestone**: M6
- **Dependencies**: CF-027
- **Impacted Files**:
  - `internal/rag/citation.go`
  - `internal/rag/citation_test.go`
- **Description**: Transform retrieved chunks into verifiable, structured citation objects containing `source_id`, `file_path`, `start_line`, `end_line`, `similarity`, and context snippets. Format system prompt with numbered references `[1]`, `[2]`.
- **Acceptance Criteria**:
  - Prompt enforces strict citation requirement.
  - Output maps citations back to exact source file lines.
- **Verification Command**:
  ```bash
  go test -v ./internal/rag/citation_test.go
  ```
- **Status**: `DONE`

---

### `CF-029`: RAG Generation Service
- **Milestone**: M6
- **Dependencies**: CF-017, CF-028
- **Impacted Files**:
  - `internal/service/rag_service.go`
  - `internal/service/rag_service_test.go`
- **Description**: Orchestrate end-to-end query flow: embed query -> hybrid retrieval -> format prompt with context chunks -> call `LLMProvider` -> parse output and return citations.
- **Acceptance Criteria**:
  - Unit tests with mock provider and mock vector repo verify complete query pipeline.
- **Verification Command**:
  ```bash
  go test -v ./internal/service/rag_service_test.go
  ```
- **Status**: `DONE`

---

### `CF-030`: Server-Sent Events (SSE) Streaming Chat Handler
- **Milestone**: M6
- **Dependencies**: CF-029
- **Impacted Files**:
  - `internal/api/handler/chat.go`
  - `internal/api/handler/chat_test.go`
- **Description**: Implement `POST /api/v1/projects/:id/chat/completions/stream`. Stream `event: citation`, `event: message`, and `event: done` chunks with standard SSE formatting.
- **Acceptance Criteria**:
  - Returns `Content-Type: text/event-stream`.
  - Flushes output after each token chunk.
  - Test client parses stream events without dropping frames.
- **Verification Command**:
  ```bash
  go test -v ./internal/api/handler/chat_test.go
  ```
- **Status**: `DONE`

---

## Milestone 7: Web Frontend & Developer UI

### `CF-031`: Next.js 14 App Router & shadcn/ui Scaffold
- **Milestone**: M7
- **Dependencies**: None
- **Impacted Files**:
  - `web/package.json`
  - `web/tsconfig.json`
  - `web/tailwind.config.ts`
  - `web/src/app/layout.tsx`
  - `web/src/app/page.tsx`
- **Description**: Initialize Next.js 14 App Router with TypeScript, Tailwind CSS, Lucide icons, and shadcn/ui components (Button, Card, Input, Dialog, Table, Badge).
- **Acceptance Criteria**:
  - `pnpm build` succeeds with zero TypeScript or ESLint errors.
- **Verification Command**:
  ```bash
  cd web && pnpm build
  ```
- **Status**: `DONE`

---

### `CF-032`: API Client & Typed Schema Integration
- **Milestone**: M7
- **Dependencies**: CF-031
- **Impacted Files**:
  - `web/src/lib/api.ts`
  - `web/src/types/api.ts`
- **Description**: Implement type-safe fetch client for `/api/v1` generated from `openapi.yaml`. Support credentials cookie, authorization header, and typed error handling.
- **Acceptance Criteria**:
  - Type-safe functions for projects, sources, jobs, and auth.
- **Verification Command**:
  ```bash
  cd web && pnpm typecheck
  ```
- **Status**: `DONE`

---

### `CF-033`: Project Dashboard & Source Ingestion Manager UI
- **Milestone**: M7
- **Dependencies**: CF-032
- **Impacted Files**:
  - `web/src/app/projects/page.tsx`
  - `web/src/app/projects/[id]/page.tsx`
  - `web/src/components/source-list.tsx`
  - `web/src/components/add-source-dialog.tsx`
  - `web/src/components/sync-status-badge.tsx`
- **Description**: Build project management interface. Display repository sync status, active ingestion progress bars, document counts, and sync trigger actions.
- **Acceptance Criteria**:
  - UI renders project details, polls ingestion job status, and handles errors cleanly.
- **Verification Command**:
  ```bash
  cd web && npm run build
  ```
- **Status**: `DONE`

---

### `CF-034`: Interactive RAG Query & Streaming Chat UI
- **Milestone**: M7
- **Dependencies**: CF-033
- **Impacted Files**:
  - `web/src/app/projects/[id]/chat/page.tsx`
  - `web/src/components/chat-box.tsx`
  - `web/src/components/citation-viewer.tsx`
  - `web/src/components/code-snippet.tsx`
- **Description**: Build interactive AI chat interface. Support real-time token streaming via SSE, render syntax-highlighted code blocks, and provide clickable citations opening exact file line previews.
- **Acceptance Criteria**:
  - Renders streamed tokens in real-time.
  - Clicking citation highlights file path, start line, and end line snippet in modal.
- **Verification Command**:
  ```bash
  cd web && npm run build
  ```
- **Status**: `DONE`

---

## Milestone 8: Security Hardening, E2E & Production Readiness

### `CF-035`: Cross-Project Data Isolation Integration Test
- **Milestone**: M8
- **Dependencies**: CF-016, CF-026, CF-029
- **Impacted Files**:
  - `tests/integration/isolation_test.go`
- **Description**: Create automated integration test with two distinct synthetic users and projects. Seed proprietary code chunks into Project A. Execute broad semantic RAG queries from Project B.
- **Acceptance Criteria**:
  - Query from Project B returns 0 chunks and 0 citations from Project A.
  - Ent relational queries reject Project A IDs with 404/403.
- **Verification Command**:
  ```bash
  go test -v ./tests/integration/...
  ```
- **Status**: `DONE`

---

### `CF-036`: Rate Limiting & Token Bucket Middleware
- **Milestone**: M8
- **Dependencies**: CF-004
- **Impacted Files**:
  - `internal/api/middleware/ratelimit.go`
  - `internal/api/middleware/ratelimit_test.go`
- **Description**: Implement Redis-backed sliding window rate limiter. Enforce 100 req/min for general API, 20 req/min for RAG chat generation, and 10 req/min for repository sync triggers.
- **Acceptance Criteria**:
  - Excess requests receive 429 Too Many Requests with `Retry-After` header.
- **Verification Command**:
  ```bash
  go test -v ./internal/api/middleware/ratelimit_test.go
  ```
- **Status**: `DONE`

---

### `CF-037`: Full End-to-End System Test Suite
- **Milestone**: M8
- **Dependencies**: All previous tasks
- **Impacted Files**:
  - `tests/e2e/e2e_test.go`
- **Description**: Complete end-to-end user journey test: create user -> create project -> connect source repo -> run worker ingestion -> verify chunks in pgvector -> query via SSE chat -> assert correct citation line numbers -> delete project and verify cascading purge.
- **Acceptance Criteria**:
  - All steps pass without manual intervention.
  - DB is verified clean after test completion.
- **Verification Command**:
  ```bash
  go test -v ./tests/e2e/...
  ```
- **Status**: `DONE`

---

### `CF-038`: Docker Container Hardening & Release Artifacts
- **Milestone**: M8
- **Dependencies**: CF-037
- **Impacted Files**:
  - `Dockerfile.api`
  - `Dockerfile.worker`
  - `Dockerfile.web`
  - `.github/workflows/release.yml`
- **Description**: Multi-stage minimal Docker builds using unprivileged scratch/distroless base images. Run as non-root UID 10001. Enforce read-only root filesystems where applicable.
- **Acceptance Criteria**:
  - Container images pass `govulncheck` and Trivy vulnerability scans with 0 critical issues.
- **Verification Command**:
  ```bash
  make security && docker build -t contextforge-api:test -f Dockerfile.api .
  ```
- **Status**: `DONE`

---

## Milestone 9: External Relational Database Knowledge Sources

### `CF-039`: Database Migration & Ent Schema for Multi-Source
- **Milestone**: M9
- **Dependencies**: CF-005, CF-006
- **Impacted Files**:
  - `migrations/000003_add_database_sources.up.sql`
  - `migrations/000003_add_database_sources.down.sql`
  - `internal/ent/schema/source.go`
  - `internal/ent/schema/database_source.go`
  - `internal/ent/schema/project.go`
- **Description**: Relax GitHub-only columns (`repo_owner`, `repo_name`) on `sources`, create `database_sources` table referencing `sources(id)` and `projects(id)`, with AES-256-GCM encrypted connection strings.
- **Status**: `DONE`

---

### `CF-040`: Pure Go Database Connector Framework & SSRF Guard
- **Milestone**: M9
- **Dependencies**: CF-039
- **Impacted Files**:
  - `internal/connector/model.go`
  - `internal/connector/registry.go`
  - `internal/connector/ssrf.go`
  - `internal/connector/sensitive.go`
  - `internal/connector/postgres.go`
  - `internal/connector/mysql.go`
  - `internal/connector/sqlite.go`
  - `internal/connector/mssql.go`
- **Description**: Pure Go connectors (zero CGO) for PostgreSQL, CockroachDB, MySQL, MariaDB, SQLite, and SQL Server. SSRF target filtering with IPv6/CGNAT normalization, DNS pre-checks, and sensitive column tokenization.
- **Status**: `DONE`

---

### `CF-041`: Knowledge Normalizer (DDL & Sample Row Documents)
- **Milestone**: M9
- **Dependencies**: CF-040
- **Impacted Files**:
  - `internal/service/knowledge_normalizer.go`
  - `internal/service/knowledge_normalizer_test.go`
- **Description**: Generates deterministic Markdown table DDL documents with line anchors (`schema/{schema}/{table}.sql`) and sample row batch tables (`data/{schema}/{table}.sql`) with SHA-256 content hashes.
- **Status**: `DONE`

---

### `CF-042`: Database Ingestion Worker Pipeline & Vector Purge
- **Milestone**: M9
- **Dependencies**: CF-041, CF-021
- **Impacted Files**:
  - `internal/worker/database_ingest_handler.go`
  - `internal/worker/database_ingest_handler_test.go`
  - `internal/queue/tasks.go`
  - `cmd/worker/main.go`
- **Description**: Asynchronous Redis Asynq task handler `database:sync`. Executes schema introspection, document normalization, chunking, embedding, obsolete vector chunk purging, and atomic upserts.
- **Status**: `DONE`

---

### `CF-043`: REST API Endpoints & Multi-Tenant Isolation
- **Milestone**: M9
- **Dependencies**: CF-042, CF-008
- **Impacted Files**:
  - `internal/service/database_source_service.go`
  - `internal/api/handler/database_source.go`
  - `internal/api/handler/database_source_test.go`
  - `internal/api/server.go`
  - `docs/api/openapi.yaml`
- **Description**: 10 REST endpoints under `/api/v1/projects/:id/sources/database` covering live ping test, creation, masked listings, updates, deletion, catalog metadata inspection, and sync triggers. Scoped strictly by `project_id`.
- **Status**: `DONE`

---

### `CF-044`: Next.js Web UI Integration (Add Modal & Source Manager)
- **Milestone**: M9
- **Dependencies**: CF-043, CF-033
- **Impacted Files**:
  - `web/src/components/add-database-source-modal.tsx`
  - `web/src/app/projects/[id]/page.tsx`
  - `web/src/lib/api.ts`
  - `web/src/types/api.ts`
- **Description**: Multi-step modal for adding database sources with live connection test, engine selection, sensitive column warnings, and project overview database source list with sync triggers and spinners.
- **Status**: `DONE`

