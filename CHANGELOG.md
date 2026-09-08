# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Comprehensive engineering specification ([SPEC.md](SPEC.md)) updated with all 20 production requirements.
- Architecture documentation, STRIDE threat model, and RAG design specifications.
- Architecture Decision Records (ADR-0001 through ADR-0010) establishing foundational technical choices.
- Standardized Ent ORM data access layer with versioned SQL migrations.
- Support for Bring Your Own Key (BYOK) cloud providers (OpenAI, Gemini, Anthropic) and local developer CLI subscription bridges (`opencode`, `claudecode`, `gemini cli`, `codex`).
- Decoupled zero-cost local embedding architecture (Ollama / in-process).
- GitHub Personal Access Token (PAT) fallback mode for rapid local experimentation alongside GitHub App.
- Docker Compose development environment supporting both Full-Container and Host-CLI Bridge profiles.
- External Database Knowledge Sources feature supporting PostgreSQL, CockroachDB, MySQL, MariaDB, SQLite (zero CGO), and Microsoft SQL Server.
- Architecture Decision Record [ADR-0011](docs/adr/0011-database-knowledge-sources.md) for external relational database knowledge sources.
- Pure Go database connector framework with SSRF target filtering, pre-connection DNS validation, IPv6/CGNAT normalization, and sensitive column tokenization.
- Knowledge normalizer generating deterministic SQL DDL documents and Markdown row tables with 1-indexed line anchors for hybrid vector retrieval.
- AES-256-GCM authenticated credential encryption at rest with strict base64 decoding.
- Asynchronous database sync worker pipeline on Redis Asynq with obsolete vector chunk purging and content hash diffing.
- 10 REST API endpoints under `/api/v1/projects/:id/sources/database` with multi-tenant isolation.
- Next.js 14 frontend integration with `AddDatabaseSourceModal` (live test ping, engine selection, extraction modes) and project database source management with status spinners.
- Granular implementation task backlog tracking `CF-001` through `CF-044` across 9 milestones.

