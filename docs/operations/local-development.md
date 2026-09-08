# Local Development Guide

ContextForge supports two primary local development workflows:
1. **Hybrid Host-CLI Bridge Profile (Recommended)**: Runs stateful dependencies (PostgreSQL 16 + pgvector, Redis 7) in Docker, while running the Go API, workers, and Next.js frontend directly on the host machine. This enables zero-friction access to host-installed coding agent CLIs (`opencode`, `claudecode`, `gemini`, `codex`), local keychain credentials, and host-bound Ollama instances.
2. **Full Container Profile**: Runs all services (PostgreSQL, Redis, API, worker, frontend) inside Docker Compose containers for reproducible environments or headless testing.

---

## 1. Prerequisites

Ensure the following tools are installed on your workstation:

| Component | Minimum Version | Installation Command / Link |
| :--- | :--- | :--- |
| **Go** | 1.23+ | `brew install go` or [golang.org/dl](https://golang.org/dl) |
| **Node.js** | 20 LTS | `brew install node@20` or [nodejs.org](https://nodejs.org) |
| **pnpm / npm** | 9+ (or npm 10+) | `corepack enable` or `npm install -g pnpm` |
| **Docker & Docker Compose** | v2.20+ | Docker Desktop or Colima / OrbStack |
| **golang-migrate** | v4.17+ | `brew install golang-migrate` |
| **golangci-lint** | v1.60+ | `brew install golangci-lint` |
| **Air** (Optional hot-reload) | v1.50+ | `go install github.com/air-verse/air@latest` |
| **OpenCode CLI** (Optional) | Latest | Installed with active OpenCode Go subscription |
| **Claude Code CLI** (Optional) | Latest | `npm install -g @anthropic-ai/claude-code` |
| **Gemini CLI** (Optional) | Latest | Installed via Google Cloud SDK / NPM |
| **Ollama** (Optional for local AI) | Latest | `brew install ollama` or [ollama.com](https://ollama.com) |

---

## 2. Environment Setup

ContextForge reads configuration from environment variables. An annotated template is provided in `.env.example`.

```bash
# 1. Clone the repository
git clone https://github.com/your-org/contextforge.git
cd ContextForge

# 2. Copy the example environment file
cp .env.example .env

# 3. Generate secure cryptographic secrets for session tokens and encryption
# 32-byte hex for AES-256-GCM token encryption:
openssl rand -hex 32

# 64-byte hex for JWT / session secret:
openssl rand -hex 64
```

Edit `.env` and set:
- `CF_AUTH_TOKEN_ENCRYPTION_KEY`: Set to the 64-character hex string generated above (32 bytes).
- `CF_AUTH_JWT_SECRET`: Set to the 128-character hex string generated above (64 bytes).
- For local testing without GitHub OAuth App credentials, supply `CF_GITHUB_PAT_FALLBACK` with a personal access token (classic or fine-grained with `repo` scope).

---

## 3. Hybrid Development Workflow (Host + Docker)

This mode gives your local Go binaries full access to host CLI agents and local Ollama while keeping database administration clean.

### Step 3.1: Start PostgreSQL and Redis in Docker
```bash
# Spin up Postgres with pgvector and Redis in background
make dev-deps
# Alternatively: docker compose up -d postgres redis
```

Verify that the containers are healthy:
```bash
docker compose ps
```

Verify `pgvector` and `uuid-ossp` extensions:
```bash
docker compose exec postgres psql -U contextforge -d contextforge -c "\dx"
```
Expected output should list `uuid-ossp` and `vector`.

### Step 3.2: Run Database Migrations
```bash
# Run up migrations against the local postgres instance
make migrate-up
```

### Step 3.3: Run the Go API Server
In a dedicated terminal tab:
```bash
# Using standard Go runner:
go run cmd/api/main.go

# OR with live hot-reload using Air:
air -c .air.toml
```
The API server starts on `http://localhost:8080`. Health check is at:
```bash
curl http://localhost:8080/healthz
```

### Step 3.4: Run the Background Worker
In another terminal tab:
```bash
go run cmd/worker/main.go
```
The worker connects to Redis, starts Asynq queue processing, and polls for repository synchronization and document embedding tasks.

### Step 3.5: Run the Next.js Frontend
In a third terminal tab:
```bash
cd web
pnpm install  # or npm install
pnpm dev      # or npm run dev
```
Open `http://localhost:3000` in your web browser.

---

## 4. Local CLI Subscription Integration

ContextForge can route LLM completion requests through coding agent CLIs running directly on the host machine.

### OpenCode CLI Integration
1. Verify `opencode` is authenticated on host:
   ```bash
   opencode whoami
   ```
2. In `.env`:
   ```bash
   CF_LLM_PROVIDER_DEFAULT="cli_opencode"
   CF_CLI_OPENCODE_PATH="/usr/local/bin/opencode" # or output of `which opencode`
   ```

### Claude Code CLI Integration
1. Verify `claude` is authenticated on host:
   ```bash
   claude --version
   ```
2. In `.env`:
   ```bash
   CF_CLI_CLAUDE_PATH="claude"
   ```

### Ollama Local Embeddings Integration
1. Start Ollama and pull the recommended embedding model:
   ```bash
   ollama serve &
   ollama pull nomic-embed-text
   ```
2. In `.env`:
   ```bash
   CF_EMBEDDING_PROVIDER_DEFAULT="ollama"
   CF_EMBEDDING_OLLAMA_BASE_URL="http://localhost:11434"
   CF_EMBEDDING_OLLAMA_MODEL="nomic-embed-text"
   CF_EMBEDDING_DIMENSIONS=768
   ```

---

## 5. Full Container Profile

To run everything in Docker containers:
```bash
# Start all services (postgres, redis, api, worker, web)
docker compose --profile full up -d --build

# View container logs
docker compose logs -f api worker
```

> [!NOTE]
> In container mode, host CLI tools (`opencode`, `claude`) are not directly accessible unless mounted into the container or exposed via an HTTP wrapper. Use cloud BYOK API keys (OpenAI, Anthropic, Gemini) or containerized Ollama when running the full container profile.

---

## 6. Testing and Quality Checks

Run all verification tools before pushing changes:

```bash
# Run unit tests
make test

# Run tests with race detector and coverage report
make test-coverage

# Run linters
make lint

# Run Go security audit
make security

# Check database migration status
make migrate-version
```

---

## 7. Clean Up & Reset

```bash
# Stop containers and keep volumes
docker compose down

# Stop containers and erase database volumes (Full Reset)
make reset-db
```
