# Contributing to ContextForge

Thank you for your interest in contributing to ContextForge!

## Development Philosophy

ContextForge is engineered with strict production standards:
- **API First**: The web client communicates strictly via the documented REST API.
- **Zero In-Memory Filtering for Tenancy**: Project isolation is a mandatory database constraint enforced during queries, never post-filtered in memory.
- **No Mock Implementations in Production**: Features must be fully functional and tested.
- **Architecture Decision Records (ADRs)**: All significant design decisions are documented in `docs/adr/`.

## Getting Started

1. Fork and clone the repository.
2. Ensure you have Go 1.22+, Node.js 20+, and Docker installed.
3. Copy `.env.example` to `.env`.
4. Run `make setup` to download Go modules and install npm dependencies.
5. Run `make dev-infra` to start PostgreSQL and Redis in Docker.
6. Run `make migrate-up` to apply database migrations.

## Pull Request Checklist

Before submitting a pull request, verify:

- [ ] Code is formatted: `make format`
- [ ] Linters pass: `make lint`
- [ ] Unit tests pass: `make test-unit`
- [ ] Integration tests pass: `make test-integration`
- [ ] Cross-project isolation test passes: `make test-isolation`
- [ ] Security checks pass: `make security`
- [ ] New endpoints are documented in `docs/api/openapi.yaml`
- [ ] Corresponding ADR is created or updated if architectural patterns changed
