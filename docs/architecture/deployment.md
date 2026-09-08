# ContextForge Deployment Guide

ContextForge is self-hostable and supports multiple deployment architectures:

## 1. Local Developer Profile (Host-CLI Bridge)

Recommended when using host-installed CLI subscriptions like `opencode`, `claudecode`, `gemini cli`, or `codex`:
- **Docker**: Runs PostgreSQL 16 (`pgvector/pgvector:pg16`) on port `5432` and Redis 7 on port `6379`.
- **Host**: Runs Go API (`PORT=8080`), Worker, and Next.js (`PORT=3000`).
- **Command**:
  ```bash
  make dev-infra     # Starts Postgres + Redis
  make migrate-up    # Runs Ent / SQL migrations
  make dev-api       # Starts API
  make dev-worker    # Starts background worker
  make dev-web       # Starts Next.js dev server
  ```

## 2. Docker Compose (Full Containerized Stack)

Recommended for single-node self-hosting, staging, and environments using BYOK cloud API keys:
- Runs all 5 services in Docker: `postgres`, `redis`, `api`, `worker`, `web`.
- **Command**:
  ```bash
  docker compose up --build
  ```

## 3. Production Deployment (Kubernetes / Cloud VM)

- **Database**: Managed PostgreSQL (e.g. AWS RDS / Supabase / Neon) with `pgvector` extension enabled.
- **Cache/Queue**: Managed Redis (e.g. AWS ElastiCache / Redis Cloud).
- **Compute**: Containerized `api`, `worker`, and `web` containers deployed behind an HTTPS reverse proxy (Traefik, Nginx, Caddy, or AWS ALB).
- **Secrets Management**: Secrets injected via environment variables or secret managers (HashiCorp Vault, AWS Secrets Manager).
- **Migrations**: Automated pre-deployment job running `bin/migrate up`.
