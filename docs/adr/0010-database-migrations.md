# ADR 0010: Versioned Database Migrations with golang-migrate

## Status
Accepted

## Date
2026-09-08

## Context
Production databases require predictable, auditable, reversible, and zero-downtime schema evolution. Schema management must accommodate both standard relational DDL (tables, foreign keys, unique constraints) and PostgreSQL-specific extensions and vector indexes (`CREATE EXTENSION "vector"`, `CREATE INDEX ... USING hnsw (embedding vector_cosine_ops)`).

We evaluated migration strategies:
1. **Runtime Auto-Migration (e.g. `client.Schema.Create(ctx)` at startup)**: Risky in production clusters where multiple pods start simultaneously and race on DDL locks. Does not support non-relational extension commands or rolling schema transitions.
2. **Versioned Up/Down SQL Migrations (`golang-migrate/migrate/v4`)**: Industry standard. Deterministic sequential files (`000001_name.up.sql`, `000001_name.down.sql`), tracks schema state in `schema_migrations`, works via CLI or embedded Go library, language-agnostic.
3. **Atlas Declarative Migrations**: Powerful modern schema diffing tool that can compute SQL diffs from Ent schemas.

## Decision
We chose a hybrid workflow:
1. **Runtime & CI Execution via `golang-migrate`**:
   - All migrations are stored as sequential SQL files in `migrations/`:
     - `000001_initial_schema.up.sql` / `000001_initial_schema.down.sql`
     - `000002_add_hnsw_indexes.up.sql` / `000002_add_hnsw_indexes.down.sql`
   - Executed via `make migrate-up` or an entrypoint migration job prior to API container deployment.
   - `golang-migrate` library is also embedded into the Go API binary for automated integration test database setups.
2. **Schema Diff Generation via Ent & Atlas**:
   - Developers define entities in Go using Ent schemas.
   - Atlas computes clean, versioned `.up.sql` diffs, which developers inspect, adjust (adding pgvector index tuning if necessary), and commit to version control.
3. **Strict Ban on Production Auto-Migrate**:
   - Auto-migration during API server boot is disabled in production environments.

## Consequences

### Positive
- 100% reproducible database states across local workstations, CI test containers, staging, and production.
- Full support for raw SQL extensions (`uuid-ossp`, `vector`) and HNSW index parameters.
- Reversible changes via paired `.down.sql` scripts.
- Zero race conditions during multi-instance API deployments.

### Negative
- Developers must generate and commit migration files whenever Ent schemas are modified.
- Destructive changes (column drops) require a two-phase deployment (phase 1: code ignores column; phase 2: migration drops column).

## Alternatives Considered
- **Ent Auto-Migrate in Production**: Rejected due to DDL race conditions during multi-replica container startups and inability to manage `pgvector` HNSW parameters.
- **Goose / Flyway**: Goose is good, but `golang-migrate` has broader CI ecosystem adoption, excellent Postgres driver support, and seamless CLI tools.

## References
- SPEC.md: Section 21 (Database Architecture) & Section 48 (Database Migrations)
- [golang-migrate Documentation](https://github.com/golang-migrate/migrate)
- [Atlas Migration Tool](https://atlasgo.io)
