# ContextForge Data Flow Architecture

## 1. Authentication & Session Flow

1. User initiates GitHub OAuth login (`GET /api/v1/auth/github/start`).
2. Server generates a cryptographically secure random `state` (stored in encrypted temporary cookie) and redirects to GitHub.
3. User authorizes; GitHub redirects back to `/api/v1/auth/github/callback`.
4. Server validates `state`, exchanges authorization code for user access token.
5. Server creates or updates the `User` record in PostgreSQL via Ent.
6. Server generates a 256-bit cryptographically random session token (`crypto/rand`).
7. Server stores the **SHA-256 hash** of the token in the `sessions` table (never plaintext).
8. Server sends the raw session token to the browser in an `HttpOnly`, `Secure`, `SameSite=Lax` cookie.

## 2. Ingestion Data Flow

### 2.1 GitHub Repository Ingestion Flow
1. User requests adding a source (e.g. GitHub repo `owner/repo`) to `Project A`.
2. API handler validates input, verifies user owns `Project A`, and creates a `ProjectSource` record.
3. API enqueues an ingestion task onto Redis (`queue:default`) and responds `202 Accepted` with `job_id`.
4. Worker picks up job:
   - Acquires Redis distributed mutex `lock:repo:{github_repo_id}` to prevent concurrent redundant clones.
   - Discovers repository tree using GitHub Connector.
   - Computes SHA-256 content hashes for all discovered files.
   - Compares hashes against stored documents; unchanged files are skipped.
   - For changed/new files: parses text, splits into semantic/code chunks, and creates embedding batches.
   - Invokes `Embedder.Embed(ctx, batch)`.
   - In an atomic transaction: stores document metadata via Ent and inserts chunks via `VectorRepository.UpsertChunks(ctx, chunks)`.
   - Releases lock and updates job status to `completed`.

### 2.2 External Relational Database Ingestion Flow
1. User tests and adds an external database source (PostgreSQL, CockroachDB, MySQL, MariaDB, SQLite, SQL Server) via `POST /api/v1/projects/:id/sources/database`.
2. API validates the connection string, enforces SSRF network target validation, tests connectivity, encrypts credentials via AES-256-GCM, and creates `Source` and `DatabaseSource` records.
3. API enqueues an asynchronous sync task onto Redis (`queue:database:sync`, task `database:sync`) and responds `201 Created`.
4. Worker picks up the database sync job:
   - Decrypts connection credentials and connects to the target external database.
   - Introspects catalog metadata: schemas, tables, views, columns, nullability, defaults, primary keys, foreign keys, and indexes.
   - If configured in `schema_and_data` mode: executes sensitive column tokenization (filtering out passwords, secrets, tokens, PII), and extracts bounded sample row batches.
   - Passes catalog metadata to `KnowledgeNormalizer`, generating deterministic SQL DDL and Markdown row documents with line-anchored syntax (`schema/{schema}/{table}.sql`).
   - Computes SHA-256 content hashes to identify schema/data modifications.
   - Chunks normalized SQL documents with line anchors, embeds chunks via `Embedder.EmbedDocuments`, purges obsolete chunks for existing modified tables, and atomically upserts vector chunks via `VectorRepository.UpsertChunks`.
   - Updates source sync status to `synced` and database source status to `ready`.

## 3. Query & RAG Retrieval Flow

1. User submits question to `/api/v1/projects/{projectID}/chat`.
2. Middleware verifies session and confirms user access to `projectID`.
3. Server embeds the question into a vector using configured `Embedder`.
4. `VectorRepository.SearchSimilar` executes a cosine distance vector search (`<=>`) in PostgreSQL strictly filtered by `project_id = $1`.
5. Retrieved chunks are scored and formatted into prompt context markers (`[SOURCE-1]`, `[SOURCE-2]`).
6. Generator (cloud API or local CLI subscription bridge `opencode`) generates streaming answer tokens.
7. Markers in the answer text are verified and mapped to structured citation metadata returned to the user.
