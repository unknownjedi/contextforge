# ADR 0007: Ent ORM as Primary Relational Database Access Layer

## Status
Accepted

## Date
2026-09-08

## Context
ContextForge requires a robust, type-safe, and maintainable database access layer to manage complex entity relationships across users, projects, sources, documents, chunks, ingestion jobs, and audit logs.

We evaluated three Go database access approaches:
1. **Ent (`entgo.io/ent`)**: Graph-based ORM developed by Meta/Ariga. Schemas defined strictly as Go code, 100% type-safe generated code, zero reflection at runtime, native edge traversals, transaction hooks, and interceptors.
2. **GORM (`gorm.io/gorm`)**: Popular Go ORM, but relies heavily on runtime reflection, uses stringly-typed column references (`db.Where("name = ?", name)`), easily masks typos until runtime, and exhibits non-deterministic preload behaviors.
3. **sqlc (`github.com/sqlc-dev/sqlc`)**: Generates type-safe Go from raw SQL queries. Extremely fast and clean, but composing complex dynamic filters (such as multi-predicate search filters with optional pagination and ordering) requires significant boilerplate or fragmented query variations.

## Decision
We selected **Ent (`entgo.io/ent`)** as the primary relational ORM for ContextForge.

Key architectural drivers:
- **Schema as Code**: Data models, field constraints, default values, and relational edges are declared in Go structs under `internal/ent/schema/`.
- **Compile-Time Type Safety**: All queries, predicates, mutations, and edge traversals are generated as strongly-typed Go code. Typos in column names or mismatched types are caught at compile time.
- **Middleware & Interceptors**: Ent hooks allow seamless injection of audit logging, timestamps, and tenant isolation assertions across all CRUD operations.
- **Atlas & golang-migrate Integration**: Ent integrates with Atlas to compute clean, reproducible SQL diffs, which are exported directly into versioned SQL files for `golang-migrate`.
- **Zero Runtime Reflection**: Unlike GORM, Ent's query execution does not rely on empty interface reflections, guaranteeing predictable runtime performance and minimal memory allocations.

## Consequences

### Positive
- Compile-time verification of all relational queries and edge relationships.
- Clean and fluent Go API: `client.Project.Query().Where(project.OwnerUserIDEQ(userID)).WithSources().All(ctx)`.
- Eliminates manual scanning boilerplate while retaining high performance.
- Automated migration diffing against actual target database states.

### Negative
- Code generation step (`go generate ./internal/ent`) is required whenever schemas change.
- Native `pgvector` operators (`<=>`, `<->`) are not part of standard Ent predicates, which reinforces our decision to isolate vector queries behind a dedicated `VectorRepository` (see ADR 0009).

## Alternatives Considered
- **GORM**: Rejected due to reflection overhead, runtime errors, and poor edge traversal ergonomics.
- **Raw SQL / sqlc**: While excellent for static queries, composing dynamic entity queries with relational edges in sqlc is significantly more complex than Ent's fluent builder.

## References
- SPEC.md: Section 1 (Executive Summary) & Section 21 (Database Architecture)
- [Ent Documentation](https://entgo.io)
