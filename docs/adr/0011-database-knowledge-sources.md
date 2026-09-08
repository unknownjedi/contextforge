# ADR 0011: External Database Knowledge Sources Architecture & Security

## Status
Accepted

## Date
2026-09-08

## Context
Code repositories often rely on relational databases whose schemas, foreign-key topologies, indexes, and sample records provide critical context for code generation, bug fixing, and query synthesis. Previously, ContextForge supported only Git repository sources (GitHub). 

To empower developers and AI coding agents with deep database awareness, ContextForge required an enterprise-grade external Database Knowledge Source capability supporting:
1. Multi-engine support: PostgreSQL, CockroachDB, MySQL, MariaDB, SQLite (zero CGO), and Microsoft SQL Server.
2. Strong security: SSRF prevention, AES-256-GCM credential encryption at rest, automatic sensitive column exclusion, error redaction, and project-scoped isolation.
3. High-fidelity RAG ingestion: Normalizing relational catalogs into deterministic Markdown/DDL knowledge documents and sample row batches with precise table and line anchors for pgvector hybrid search.

## Decision
We implemented a pluggable Database Connector and Knowledge Normalizer framework integrated into the asynchronous worker pipeline:

1. **Pure Go Driver Stack**:
   - PostgreSQL & CockroachDB: `github.com/jackc/pgx/v5` via standard `database/sql` driver (`stdlib`).
   - MySQL & MariaDB: `github.com/go-sql-driver/mysql`.
   - SQLite: `modernc.org/sqlite` (pure Go, zero CGO requirement for cross-compilation and lightweight Alpine containers).
   - SQL Server (MSSQL): `github.com/microsoft/go-mssqldb`.
   - All driver licenses are Apache 2.0 compatible (MIT, BSD-3-Clause, MPL-2.0).

2. **Security & Cryptography**:
   - **Credential Encryption at Rest**: `connection_url` strings are encrypted using AES-256-GCM with a master encryption key (`ENCRYPTION_KEY`), 96-bit cryptographic nonces, and authenticated ciphertext verification.
   - **Credential Masking**: Stored connection URLs are never exposed via REST APIs, logs, or metrics. Error sanitizers strip embedded username/password credentials.
   - **SSRF Validation**: Network targets are verified prior to connection. Private IP addresses (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `169.254.0.0/16`, `127.0.0.0/8`) and AWS/GCP metadata endpoints (`169.254.169.254`) are blocked unless explicitly exempted in local development (`ALLOW_PRIVATE_IPS=true`).
   - **Sensitive Column Filtering**: Automatic pattern matching identifies and drops columns containing secrets, tokens, password hashes, social security numbers, and payment details.

3. **Knowledge Normalization & Vector RAG Pipeline**:
   - Each table is transformed into a structured SQL/Markdown document detailing schemas, column types, nullability, defaults, primary keys, foreign keys, and indexes.
   - Deterministic SHA-256 content hashes prevent duplicate re-indexing when schemas have not changed.
   - Row batching samples representative records (configurable limit, default 50 per table) formatted into clean Markdown tables.
   - Ingestion runs asynchronously via Redis Asynq workers (`database:sync` priority task) using standard AST chunkers, generating vector embeddings in pgvector with cosine similarity distance.

4. **REST API & Web UI**:
   - 10 dedicated REST endpoints under `/api/v1/projects/:id/sources/database...`.
   - Next.js 14 interactive UI supporting live connection test, engine selection, extraction mode toggles, sync triggering, and status polling.

## Consequences

### Positive
- AI coding agents can answer complex queries about schema topologies, join paths, column data types, and realistic table contents.
- Zero CGO dependency ensures instant, reproducible container builds on any architecture (x86_64, ARM64).
- Defense-in-depth security prevents SSRF and credential leaks at all layers.
- Project-level authorization guarantees multi-tenant database isolation.

### Negative
- Asynchronous ingestion of large databases requires bounded row limits to avoid saturating vector storage.
- SQLite connections are local to the host/filesystem, requiring appropriate volume mounts or local paths.

## References
- `DB_Source.md`: External Database Knowledge Source Specification
- `docs/database-sources.md`: User and Operations Guide
- `docs/security/database-credentials.md`: Security Architecture & Threat Model
