# Operational Troubleshooting Guide

This guide details common diagnostic techniques, known error signatures, and resolution steps for ContextForge operators and developers.

---

## 1. Quick Diagnostic Checklist

When experiencing issues, run the standard diagnostics pipeline:
```bash
# 1. Check Docker container status
docker compose ps

# 2. Inspect API server logs
docker compose logs -f api --tail=100

# 3. Inspect Ingestion Worker logs
docker compose logs -f worker --tail=100

# 4. Verify PostgreSQL connectivity and extensions
docker compose exec postgres psql -U contextforge -d contextforge -c "SELECT extname, extversion FROM pg_extension;"

# 5. Verify Redis connectivity and queue sizes
docker compose exec redis redis-cli ping
docker compose exec redis redis-cli keys "asynq:*"
```

---

## 2. Common Issues and Resolutions

### 2.1 Database & Migrations

#### Issue: `dirty database version X` during migration
- **Cause**: A previous migration failed midway or the migration runner process was terminated abruptly.
- **Resolution**:
  1. Inspect the migration that failed and determine whether the SQL changes were applied.
  2. Fix the underlying SQL script or database state.
  3. Force the version back to clean state:
     ```bash
     # Example: force to version X-1 or X after manual verification
     migrate -path migrations -database "${CF_DATABASE_URL}" force <VERSION>
     ```
  4. Re-run `make migrate-up`.

#### Issue: `type "vector" does not exist` or `extension "vector" is not available`
- **Cause**: Connecting to a standard PostgreSQL image without the `pgvector` extension compiled.
- **Resolution**: Ensure you are using `pgvector/pgvector:pg16` image or have run:
  ```sql
  CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
  CREATE EXTENSION IF NOT EXISTS "vector";
  ```

#### Issue: `different vector dimensions 1536 and 768`
- **Cause**: Trying to insert or query vectors with a model dimension that doesn't match the table column configuration (e.g. comparing OpenAI `text-embedding-3-small` [1536] with Ollama `nomic-embed-text` [768]).
- **Resolution**: Verify `CF_EMBEDDING_DIMENSIONS` in `.env` matches the active provider. When changing embedding models across an entire project, all documents must be re-embedded into a project configured for that dimension.

---

### 2.2 Host-CLI Bridge & Coding Agents

#### Issue: `exec: "opencode": executable file not found in $PATH`
- **Cause**: The Go API/worker was started without `$PATH` containing the location of the `opencode` binary, or the container cannot access host binaries.
- **Resolution**:
  1. Find the absolute path to the binary on your host: `which opencode`.
  2. Specify the absolute path in `.env`:
     ```bash
     CF_CLI_OPENCODE_PATH="/usr/local/bin/opencode"
     ```
  3. Run the Go API natively on the host (`go run cmd/api/main.go`), not inside a Docker container that lacks host binary mounts.

#### Issue: `CLI bridge process timeout after 60s`
- **Cause**: The coding agent CLI encountered interactive prompts, network delays, or high load.
- **Resolution**:
  1. Verify the CLI works interactively in your host terminal.
  2. Increase timeout setting in `.env`:
     ```bash
     CF_CLI_EXECUTION_TIMEOUT=180s
     ```

---

### 2.3 Security & Cryptography

#### Issue: `cipher: message authentication failed` or `invalid AES-256-GCM ciphertext`
- **Cause**: The decryption key (`CF_AUTH_TOKEN_ENCRYPTION_KEY`) does not match the key used when encrypting the stored credentials, or the ciphertext was truncated.
- **Resolution**:
  1. Confirm that `CF_AUTH_TOKEN_ENCRYPTION_KEY` in `.env` has not changed.
  2. Ensure the key is exactly 64 hexadecimal characters (32 bytes).
  3. If the key was permanently lost, user tokens must be deleted and re-authenticated via GitHub OAuth.

---

### 2.4 GitHub Rate Limits & Authentication

#### Issue: `403 API rate limit exceeded for user / IP`
- **Cause**: Unauthenticated requests (limited to 60/hr) or personal PAT hitting the 5,000/hr threshold during large repository ingestion.
- **Resolution**:
  1. Use GitHub App authentication where possible (5,000 to 15,000 req/hr per installation).
  2. Check current GitHub rate limits:
     ```bash
     curl -H "Authorization: Bearer ${CF_GITHUB_PAT_FALLBACK}" https://api.github.com/rate_limit
     ```
  3. The ContextForge worker automatically throttles and respects `X-RateLimit-Reset` headers; allow the worker queue to drain.

---

### 2.5 Port Conflicts

| Port | Service | Resolution Command |
| :--- | :--- | :--- |
| `5432` | PostgreSQL | `lsof -i :5432` (Stop local PostgreSQL service or change host port in `docker-compose.yml` to `5433:5432`) |
| `6379` | Redis | `lsof -i :6379` (Stop local redis server or re-map port) |
| `8080` | Go API | `lsof -i :8080` (Kill colliding process or set `CF_SERVER_PORT=8081`) |
| `3000` | Next.js Frontend | `lsof -i :3000` (Set `PORT=3001 pnpm dev`) |

---

### 2.6 External Database Knowledge Sources

#### Issue: `SSRF validation failed: host resolves to private/prohibited IP`
- **Cause**: By default, ContextForge blocks connections to localhost (`127.0.0.1`), internal private networks (`10.0.0.0/8`, `192.168.0.0/16`, `172.16.0.0/12`), and cloud metadata IP (`169.254.169.254`).
- **Resolution**: In local development environments when testing against local databases on your machine or Docker network, set `ALLOW_PRIVATE_IPS=true` in your `.env` file and restart the API server. In production, this must remain `false`.

#### Issue: `unsupported database type: X`
- **Cause**: The submitted `database_type` or URL scheme is not supported.
- **Resolution**: Supported database engines are: `postgres`, `cockroachdb`, `mysql`, `mariadb`, `sqlite`, and `sqlserver` (MSSQL). Ensure your URL starts with the appropriate scheme (e.g. `postgresql://`, `mysql://`, `sqlite://`, or `sqlserver://`).

#### Issue: `sqlite connection rejected: file path in restricted system directory`
- **Cause**: The SQLite file path is pointing to or within a restricted directory such as `/root`, `/.ssh`, `/.env`, `/.git`, or `/var/run/secrets`.
- **Resolution**: Move the SQLite database file to an authorized project or data directory (e.g. `./data/mydb.sqlite` or `/tmp/mydb.sqlite`).

#### Issue: `database connection failed: SSL / TLS certificate verification failed`
- **Cause**: Connecting to remote managed database services (e.g. AWS RDS, Azure Database, Supabase) with self-signed certificates or required SSL modes.
- **Resolution**:
  - PostgreSQL: Append `?sslmode=require` or `?sslmode=verify-full`.
  - MySQL: Append `?tls=skip-verify` or `?tls=custom`.
  - SQL Server: Append `?encrypt=true&trustServerCertificate=true`.

