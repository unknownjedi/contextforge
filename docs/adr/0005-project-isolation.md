# ADR 0005: Multi-Tenant Project Data Isolation & Single-Owner v1 Architecture

## Status
Accepted

## Date
2026-09-08

## Context
ContextForge manages proprietary source code, documentation, credentials, and vector embeddings. Ensuring complete data isolation between different projects and tenants is a core security requirement.

Key considerations:
1. Version 1 is strictly **single-owner per project** (`owner_user_id`), with no team collaboration or cross-user sharing. Multi-user organizations, team roles, and collaborative projects are explicitly planned for v2.
2. Cross-project data leakage must be impossible, both in relational queries and vector similarity searches. A query originating in Project A must never retrieve document chunks from Project B, even if the semantic similarity is 1.0.

## Decision
We enforce a strict **hierarchical project tenancy model**:
1. **Ownership Constraint (v1)**:
   - Each project has exactly one owner: `projects.owner_user_id NOT NULL REFERENCES users(id)`.
   - All authorization checks verify `project.owner_user_id == authenticated_user_id`.
   - Multi-user membership tables (`project_members`, `project_roles`) are deferred to v2.
2. **Relational Isolation**:
   - Every dependent entity (`sources`, `documents`, `document_chunks`, `ingestion_jobs`, `audit_logs`, `chat_sessions`) contains a mandatory, indexed foreign key `project_id UUID NOT NULL`.
   - Cascading deletes (`ON DELETE CASCADE`) ensure complete cleanup when a project is deleted.
3. **Vector Search Isolation**:
   - Every vector similarity query against `document_chunks` must include a mandatory `WHERE project_id = $1` predicate.
   - Vector indexes are scoped or composite-indexed to ensure zero vector leakage across tenants.
4. **Automated Enforcement**:
   - An automated cross-project isolation integration test (`TestCrossProjectDataIsolation`) runs on every CI build to verify that synthetic projects cannot access each other's data or vector embeddings.

## Consequences

### Positive
- Zero risk of cross-project or cross-user context contamination.
- Clean and simple authorization logic in v1 without premature RBAC/team complexity.
- Schema is fully forward-compatible: v2 can introduce a `project_members` join table without altering the underlying `project_id` foreign key relationships on resources.

### Negative
- Developers must never write naked vector queries without `project_id` filtering (enforced via `VectorRepository` interface).
- No collaborative sharing or team viewing in v1.

## Alternatives Considered
- **Database-per-Tenant or Schema-per-Tenant**: Rejected due to high operational complexity, connection pool exhaustion, and difficult migration orchestration across thousands of potential schemas.
- **Row-Level Security (PostgreSQL RLS)**: Considered, but explicit application-layer tenancy filtering backed by composite indexing and integration tests provides better portability, clearer debugging, and identical performance with Ent ORM.

## References
- SPEC.md: Section 10 (Single-User Ownership Model), Section 64 (Project-Level Data Isolation), Section 68 (Cross-Project Isolation Verification)
