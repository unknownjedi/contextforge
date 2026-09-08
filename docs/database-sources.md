# External Database Knowledge Sources Guide

ContextForge allows connecting external relational databases directly into your project's knowledge base alongside Git repositories. Schemas, foreign keys, indexes, table comments, and sample row data are automatically extracted, normalized into structured SQL/Markdown knowledge documents, and indexed into `pgvector` with line-level citations.

---

## Supported Database Engines

| Engine | Driver / Protocol | URL Scheme | Default Port | Zero-CGO |
|---|---|---|---|:---:|
| **PostgreSQL** | `pgx/v5` stdlib | `postgresql://` or `postgres://` | `5432` | Yes |
| **CockroachDB** | `pgx/v5` stdlib (Postgres wire) | `postgresql://` or `cockroachdb://` | `26257` | Yes |
| **MySQL** | `go-sql-driver/mysql` | `mysql://` | `3306` | Yes |
| **MariaDB** | `go-sql-driver/mysql` | `mariadb://` or `mysql://` | `3306` | Yes |
| **SQLite** | `modernc.org/sqlite` | `sqlite://` or `sqlite3://` | N/A (Local) | Yes |
| **SQL Server (MSSQL)** | `microsoft/go-mssqldb` | `sqlserver://` or `mssql://` | `1433` | Yes |

All drivers are 100% pure Go without CGO dependencies, enabling clean cross-compilation and lightweight Alpine Linux container deployments.

---

## Connection String Formats

### 1. PostgreSQL & CockroachDB
```
postgresql://username:password@hostname:5432/dbname?sslmode=require
cockroachdb://username:password@hostname:26257/dbname?sslmode=verify-full
```

### 2. MySQL & MariaDB
```
mysql://username:password@hostname:3306/dbname?parseTime=true
mariadb://username:password@hostname:3306/dbname
```

### 3. SQLite
```
sqlite:///path/to/data/app.db
sqlite:///var/lib/sqlite/mydb.sqlite3?mode=ro
```

### 4. Microsoft SQL Server
```
sqlserver://username:password@hostname:1433?database=app_db&encrypt=true&TrustServerCertificate=false
mssql://username:password@hostname:1433/instance_name?database=app_db
```

---

## Extraction Modes

ContextForge supports two extraction modes per database source:

1. **`schema_only` (Default)**:
   - Queries system catalogs (`information_schema`, `sys.tables`, `sqlite_master`, etc.).
   - Extracts tables, views, columns, nullability, defaults, primary keys, foreign keys, and indexes.
   - Generates deterministic SQL DDL and Markdown documentation.
   - Zero access to user table data.

2. **`schema_and_data`**:
   - In addition to schema introspection, samples up to `max_sample_rows` (default 50, maximum 500) per table.
   - Automatically skips tables containing sensitive data or matching exclusion rules.
   - Formats records as Markdown tables chunked and embedded in vector space.

---

## Security & Defense-in-Depth

### Credential Encryption at Rest (AES-256-GCM)
Database connection strings containing credentials are never stored in plaintext. They are encrypted using AES-256-GCM authenticated encryption with 96-bit cryptographic nonces:
- Key: Configured via the `ENCRYPTION_KEY` environment variable (32 bytes).
- API responses: Connection strings are redacted and never returned to clients or logged.

### SSRF Protection
The connector framework verifies all hostnames and IP addresses before dialing:
- **Private IP Blocking**: Resolves targets and blocks private/loopback ranges:
  - `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`
  - `127.0.0.0/8` (Loopback)
  - `169.254.0.0/16` (Link-local & AWS/GCP instance metadata service `169.254.169.254`)
- **Development Override**: For local testing with docker-compose or localhost, set `ALLOW_PRIVATE_IPS=true`.

### Automatic Sensitive Column Filtering
Columns matching sensitive patterns are excluded from row data sampling:
- Patterns: `password`, `token`, `secret`, `api_key`, `private_key`, `auth`, `hash`, `salt`, `credit_card`, `card_num`, `cvv`, `ssn`, `social_security`, `pin`, `jwt`.
- Custom column exclusions can be configured per table via the `excluded_columns` setting.

### Credential Error Redaction
If a database connection error occurs, any embedded username, password, or connection token is scrubbed using regular expression redactors prior to returning the message or writing logs.

---

## Least Privilege Setup Guide

Always connect ContextForge using a dedicated read-only user with restricted permissions.

### PostgreSQL
```sql
CREATE USER contextforge_reader WITH PASSWORD 'strong_password';
GRANT CONNECT ON DATABASE my_database TO contextforge_reader;
GRANT USAGE ON SCHEMA public TO contextforge_reader;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO contextforge_reader;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO contextforge_reader;
```

### MySQL / MariaDB
```sql
CREATE USER 'contextforge_reader'@'%' IDENTIFIED BY 'strong_password';
GRANT SELECT, SHOW VIEW ON my_database.* TO 'contextforge_reader'@'%';
FLUSH PRIVILEGES;
```

### Microsoft SQL Server
```sql
CREATE LOGIN contextforge_reader WITH PASSWORD = 'strong_password';
USE app_db;
CREATE USER contextforge_reader FOR LOGIN contextforge_reader;
ALTER ROLE db_datareader ADD MEMBER contextforge_reader;
GRANT VIEW DEFINITION TO contextforge_reader;
```

---

## REST API Endpoints

All endpoints are scoped under `/api/v1/projects/:id/sources/database`:

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/test` | Test raw connection parameters before creating |
| `POST` | `/` | Create a new external database source |
| `GET` | `/` | List all database sources for the project |
| `GET` | `/:source_id` | Get database source details (masked credentials) |
| `PATCH` | `/:source_id` | Update source configuration or name |
| `DELETE` | `/:source_id` | Disconnect source and delete associated vectors |
| `POST` | `/:source_id/test` | Test stored connection using encrypted credentials |
| `GET` | `/:source_id/metadata` | Fetch live introspected schema & table catalog |
| `POST` | `/:source_id/sync` | Trigger asynchronous schema/data ingestion job |
| `GET` | `/:source_id/status` | Check current sync status and last error |

---

## Docker & Container Networking

When running ContextForge inside Docker containers:
- To connect to a database running on the **Docker host machine**:
  - macOS / Windows: Use `host.docker.internal` as the host (e.g. `postgresql://user:pass@host.docker.internal:5432/db`).
  - Linux: Set `--add-host=host.docker.internal:host-gateway` or connect via the container gateway IP.
- To connect to another container in the same Docker network:
  - Use the service name (e.g. `postgresql://user:pass@postgres:5432/contextforge`).
