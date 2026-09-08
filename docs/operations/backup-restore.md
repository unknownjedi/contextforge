# Backup and Disaster Recovery Guide

This guide details procedures for scheduled backups, point-in-time recovery (PITR), disaster recovery runbooks, and validation steps for ContextForge data stores (PostgreSQL with `pgvector` and Redis).

---

## 1. Recovery Objectives

| Metric | Target | Description |
| :--- | :--- | :--- |
| **RPO (Recovery Point Objective)** | < 15 Minutes | Maximum acceptable data loss window in disaster scenarios. |
| **RTO (Recovery Time Objective)** | < 60 Minutes | Maximum duration required to restore full production service. |

---

## 2. PostgreSQL & pgvector Backup Strategy

ContextForge relational tables and high-dimensional vector embeddings are stored inside PostgreSQL 16.

### 2.1 Logical Backups (`pg_dump`)
Run logical backups daily or prior to major software migrations. The custom directory format (`-F c`) enables parallel dumping and selective restoration.

```bash
#!/usr/bin/env bash
set -euo pipefail

BACKUP_DATE=$(date -u +"%Y%m%d_%H%M%SZ")
BACKUP_FILE="/backups/contextforge_${BACKUP_DATE}.dump"

# Dump entire database including pgvector extension and schemas
pg_dump \
  -h "${DB_HOST}" \
  -p "${DB_PORT}" \
  -U "${DB_USER}" \
  -d "${DB_NAME}" \
  -F c \
  -b \
  -v \
  -f "${BACKUP_FILE}"

# Encrypt backup before cloud storage transfer
gpg --symmetric --cipher-algo AES256 "${BACKUP_FILE}"

# Upload to S3 / GCS immutable bucket
aws s3 cp "${BACKUP_FILE}.gpg" "s3://${BACKUP_S3_BUCKET}/postgres/${BACKUP_DATE}/"
```

### 2.2 Physical Backups and WAL Archiving (PITR)
For enterprise production environments with low RPO targets, enable Continuous Archiving and Write-Ahead Log (WAL) archiving using tools like **pgBackRest** or **WAL-E / WAL-G**:

```ini
# postgresql.conf
wal_level = replica
archive_mode = on
archive_command = 'pgbackrest --stanza=contextforge archive-push %p'
archive_timeout = 300
```

---

## 3. Database Restoration Procedure

### 3.1 Restoring to a Fresh PostgreSQL Instance
1. **Provision target PostgreSQL 16 instance**:
   Ensure `pgvector` and `uuid-ossp` packages are installed in the PostgreSQL runtime.

2. **Pre-create extensions and schema**:
   ```sql
   CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
   CREATE EXTENSION IF NOT EXISTS "vector";
   ```

3. **Execute `pg_restore`**:
   ```bash
   # Download and decrypt dump
   gpg --decrypt backup.dump.gpg > backup.dump

   # Restore in parallel (e.g. 4 jobs)
   pg_restore \
     -h "${RESTORE_DB_HOST}" \
     -p "${RESTORE_DB_PORT}" \
     -U "${RESTORE_DB_USER}" \
     -d "${RESTORE_DB_NAME}" \
     -v \
     -j 4 \
     --no-owner \
     --no-acl \
     backup.dump
   ```

4. **HNSW Vector Index Validation**:
   After restoration, inspect index health and run a sample query:
   ```sql
   SELECT indexname, indexdef 
   FROM pg_indexes 
   WHERE tablename = 'document_chunks';

   -- Force index scan verification
   SET enable_seqscan = OFF;
   SELECT id, 1 - (embedding <=> '[0.01, 0.02, ...]'::vector) AS similarity
   FROM document_chunks
   WHERE project_id = 'c0000000-0000-0000-0000-000000000001'
   ORDER BY embedding <=> '[0.01, 0.02, ...]'::vector
   LIMIT 1;
   ```

> [!TIP]
> If vector indexes are corrupt or require reindexing after a major PostgreSQL version upgrade, run:
> ```sql
> REINDEX INDEX CONCURRENTLY idx_chunks_embedding_hnsw;
> ```

---

## 4. Cryptographic Key Safety & Token Recovery

> [!CAUTION]
> GitHub access tokens and user provider API keys are encrypted at rest using AES-256-GCM via `CF_AUTH_TOKEN_ENCRYPTION_KEY`.
> **If this 32-byte encryption key is lost, encrypted tokens in the database CANNOT be decrypted.**

### Key Preservation Protocol
1. Store `CF_AUTH_TOKEN_ENCRYPTION_KEY` in two independent secure hardware key vaults (e.g., AWS KMS and 1Password/HashiCorp Vault).
2. Key rotation procedure:
   - When rotating `CF_AUTH_TOKEN_ENCRYPTION_KEY`, run the ContextForge re-encryption migration utility:
     ```bash
     go run cmd/admin/main.go rotate-encryption-key \
       --old-key="${OLD_HEX_KEY}" \
       --new-key="${NEW_HEX_KEY}"
     ```

---

## 5. Redis State and Queue Recovery

Redis primarily stores ephemeral data: Asynq job queues, rate limit windows, and short-lived query caches.

1. **Persistence Mode**: Production Redis should enable both RDB snapshots (`save 900 1 300 10`) and Append-Only File (`appendonly yes`, `appendfsync everysec`).
2. **Cold Start Recovery**: If Redis suffers catastrophic loss without backup, queued ingestion jobs can be re-triggered safely because repository ingestion is idempotent:
   ```bash
   # Re-sync all active project sources via admin command
   go run cmd/admin/main.go enqueue-resync --all-active
   ```

---

## 6. Disaster Recovery Drills

Schedule automated recovery drills quarterly in an isolated staging environment:
1. Fetch latest production dump from S3.
2. Spin up ephemeral Docker Compose test container with `pgvector`.
3. Restore the dump and run schema validation tests:
   ```bash
   make test-restore DB_URL="postgres://test:test@localhost:5433/contextforge_test?sslmode=disable"
   ```
4. Verify checksums and query performance benchmarks.
