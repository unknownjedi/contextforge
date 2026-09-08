# Database Knowledge Sources — Feature Specification

**Status:** Proposed  
**Version:** 1.0  
**Feature:** Database Knowledge Sources  
**Primary Backend:** Go  
**Application Database:** PostgreSQL + pgvector  
**Scope:** Project-scoped external database connections, metadata discovery, and knowledge ingestion

---

# 1. Overview

This document specifies the implementation of database-backed knowledge sources for the RAG platform.

Users must be able to add an external database to a project by providing a database connection URL.

The application connects to the external database through a dedicated database connector abstraction, tests the connection, introspects the database schema, allows the user to select what should be indexed, and asynchronously ingests the selected information into the project's knowledge base.

The feature must be designed so additional database engines can be added without modifying the core ingestion or RAG logic.

Database sources are one type of knowledge source alongside:

- GitHub repositories
- GitHub pull requests
- GitHub issues
- URLs
- Uploaded documents
- Database sources

---

# 2. Problem Statement

Project knowledge is often distributed across application code, documentation, and the databases used by the application.

A developer may need to answer questions such as:

- What tables exist in this application?
- What columns does the `users` table contain?
- How are users related to organizations?
- Where is subscription information stored?
- Which tables contain billing information?
- What foreign-key relationships exist?
- What database schema is used by this service?
- What code interacts with the `subscriptions` table?

Without database knowledge, a RAG system only sees part of the application.

Database sources allow the RAG system to understand database structure and, when explicitly enabled, selected database records.

---

# 3. Goals

The feature must:

1. Allow users to add a database source to a project.
2. Accept a database connection URL.
3. Support multiple database engines through a connector abstraction.
4. Test database connectivity before creating or activating a source.
5. Introspect database metadata.
6. Allow users to select schemas and tables for ingestion.
7. Support schema-only ingestion.
8. Support optional row-level data ingestion.
9. Run ingestion asynchronously.
10. Store normalized knowledge in the application's PostgreSQL + pgvector database.
11. Keep database credentials encrypted.
12. Never expose credentials through the API.
13. Never write credentials to logs.
14. Keep database sources project-scoped.
15. Support the same external database being connected to multiple projects.
16. Allow different projects to use different ingestion configurations for the same external database.
17. Support incremental re-ingestion where practical.
18. Provide ingestion status and error information.
19. Allow users to manually trigger synchronization.
20. Allow users to update database source configuration.
21. Allow users to delete a database source.
22. Make adding new database engines straightforward.
23. Keep external database access read-only.

---

# 4. Non-Goals

The initial implementation must NOT attempt to:

- Modify external databases.
- Execute arbitrary SQL supplied by users.
- Provide a SQL query interface.
- Provide database administration functionality.
- Automatically ingest every database row.
- Store external database credentials in plaintext.
- Grant write access to external databases.
- Automatically discover or connect to arbitrary databases without user configuration.
- Treat an external database as the application's primary database.
- Replace the application's PostgreSQL/pgvector database.

---

# 5. Supported Databases

## 5.1 Initial Support

The architecture must support the following connectors:

### PostgreSQL

Identifier:

`postgres`

### MySQL

Identifier:

`mysql`

### MariaDB

Identifier:

`mariadb`

### SQLite

Identifier:

`sqlite`

### Microsoft SQL Server

Identifier:

`mssql`

### CockroachDB

Identifier:

`cockroachdb`

CockroachDB may reuse PostgreSQL-compatible functionality where appropriate.

---

# 6. Future Database Support

The architecture should make the following possible without redesigning the feature:

- Oracle
- MongoDB
- ClickHouse
- Snowflake
- BigQuery
- Amazon Redshift
- Elasticsearch
- OpenSearch
- Redis
- Neo4j
- Cassandra
- ScyllaDB
- DuckDB

Do not implement these unless explicitly requested.

Do not create empty placeholder connectors solely to claim support.

---

# 7. Architecture

The database feature must be separated into four logical layers:

    API
     |
     v
    Database Source Service
     |
     v
    Database Connector Registry
     |
     +-------------------------+
     |                         |
     v                         v
    PostgreSQL Connector      MySQL Connector
     |                         |
     +------------+------------+
                  |
                  v
          Database Metadata
                  |
                  v
          Knowledge Documents
                  |
                  v
            Ingestion Pipeline
                  |
                  v
               Chunking
                  |
                  v
              Embeddings
                  |
                  v
         PostgreSQL + pgvector

The database connector must not know about:

- Projects
- HTTP
- Authentication
- RAG
- LLMs
- Embeddings
- Web UI

The connector's responsibility is to communicate with an external database and expose normalized database metadata/data.

---

# 8. Suggested Go Package Structure

Use a structure similar to:

    internal/
      connectors/
        database/
          connector.go
          registry.go
          types.go

          postgres/
            connector.go
            introspection.go

          mysql/
            connector.go
            introspection.go

          mariadb/
            connector.go
            introspection.go

          sqlite/
            connector.go
            introspection.go

          mssql/
            connector.go
            introspection.go

          cockroachdb/
            connector.go
            introspection.go

      database_sources/
        service.go
        repository.go
        models.go
        validation.go

      ingestion/
        service.go
        worker.go
        pipeline.go

      knowledge/
        models.go
        repository.go

      embeddings/
        service.go

      projects/
        service.go

Exact package names may differ depending on the existing application architecture.

Do not introduce unnecessary package fragmentation.

Reuse existing source, ingestion, knowledge, authentication, authorization, job, and embedding abstractions where they already exist.

---

# 9. Database Connector Interface

Define a common connector interface.

Example:

    type DatabaseConnector interface {
        Type() string

        TestConnection(
            ctx context.Context,
            config ConnectionConfig,
        ) error

        GetMetadata(
            ctx context.Context,
            config ConnectionConfig,
        ) (*DatabaseMetadata, error)

        ListSchemas(
            ctx context.Context,
            config ConnectionConfig,
        ) ([]SchemaMetadata, error)

        ListTables(
            ctx context.Context,
            config ConnectionConfig,
            schema string,
        ) ([]TableMetadata, error)

        GetTableMetadata(
            ctx context.Context,
            config ConnectionConfig,
            schema string,
            table string,
        ) (*TableMetadata, error)

        Extract(
            ctx context.Context,
            config ConnectionConfig,
            selection ExtractionSelection,
        ) ([]KnowledgeDocument, error)
    }

The exact interface may be adjusted if the implementation reveals better abstractions.

The important requirement is that application services depend on the interface rather than a specific database implementation.

---

# 10. Connector Registry

Create a registry responsible for resolving connectors.

Example:

    type ConnectorRegistry interface {
        Get(databaseType string) (DatabaseConnector, error)
        List() []DatabaseConnector
    }

Expected mappings:

    postgres     -> PostgreSQLConnector
    mysql        -> MySQLConnector
    mariadb      -> MariaDBConnector
    sqlite       -> SQLiteConnector
    mssql        -> MSSQLConnector
    cockroachdb  -> CockroachDBConnector

The registry must reject unsupported database types.

---

# 11. Connection Configuration

The user-facing API may accept a connection URL.

Example:

    postgresql://user:password@host:5432/database

The backend must parse and validate the URL.

Do not pass unvalidated user input directly into database drivers.

A normalized internal configuration should be created.

Example:

    type ConnectionConfig struct {
        Driver   string
        Host     string
        Port     int
        Database string
        Username string
        Password string
        SSLMode  string
        Options  map[string]string
    }

Do not expose this structure directly through API responses.

---

# 12. Connection URL Support

## PostgreSQL

Support standard PostgreSQL connection URLs accepted by the selected Go PostgreSQL driver.

Example:

    postgresql://user:password@host:5432/mydb

## MySQL

Support standard MySQL connection URLs/DSNs accepted by the selected Go MySQL driver.

Example:

    mysql://user:password@host:3306/mydb

## MariaDB

Use the MySQL protocol/driver where appropriate.

Example:

    mysql://user:password@host:3306/mydb

## SQLite

Support file-based SQLite connections.

Example:

    file:///path/to/database.db

SQLite requires special handling because the application may run inside Docker.

The path must be accessible from the backend/worker runtime.

## SQL Server

Support standard SQL Server connection URL/DSN formats accepted by the selected Go driver.

## CockroachDB

Support PostgreSQL-compatible connection URLs.

---

# 13. Credential Security

This is a security-sensitive feature.

External database credentials must never be stored in plaintext.

The system must provide an encryption mechanism for secrets at rest.

Required flow:

    User enters connection URL
             |
             v
        Parse URL
             |
             v
     Extract credentials
             |
             v
       Encrypt secrets
             |
             v
       Store encrypted value

The API must never return:

- Password
- Full connection URL containing password
- API keys
- Tokens
- Certificates/private keys
- Any other secret

Example safe API response:

    {
      "id": "source_123",
      "type": "postgres",
      "name": "Production Database",
      "host": "db.example.com",
      "database": "app",
      "status": "connected"
    }

Do not return:

    {
      "connection_url": "postgresql://user:password@db.example.com/app"
    }

---

# 14. Credential Encryption Design

The encryption implementation must be configurable.

For local development, an environment-provided encryption key may be used.

For production deployments, the application should support a securely managed encryption key.

Do not hard-code encryption keys.

Do not commit encryption keys to the repository.

Document how operators configure the encryption key.

Use authenticated encryption such as AES-GCM or another well-reviewed authenticated encryption mechanism.

Each encrypted credential should use a unique nonce/IV as required by the encryption scheme.

---

# 15. Logging Requirements

Never log:

- Connection URLs
- Passwords
- Authentication tokens
- DSNs containing credentials
- TLS private keys
- Database query parameters containing secrets

Bad:

    failed connecting to postgresql://admin:supersecret@db.example.com

Good:

    failed connecting to external PostgreSQL database

Structured logs may include:

- project_id
- source_id
- database_type
- operation
- error_code

but never credentials.

---

# 16. Read-Only Requirement

The external database connector must only require read permissions.

The application must never perform:

- INSERT
- UPDATE
- DELETE
- DROP
- ALTER
- CREATE
- TRUNCATE

against an external database.

Database drivers should be used only for metadata inspection and read operations.

The documentation should strongly recommend using a dedicated read-only database account.

---

# 17. Source Model

A database source belongs to exactly one project.

Conceptually:

    Project
      |
      +--- DatabaseSource
      |
      +--- DatabaseSource
      |
      +--- GitHubSource
      |
      +--- URLSource
      |
      +--- DocumentSource

A database source should contain information conceptually equivalent to:

    id
    project_id
    type
    name
    database_type
    encrypted_connection
    configuration
    status
    created_at
    updated_at
    last_synced_at

The exact database schema should follow the existing project's conventions.

---

# 18. Same Database Across Multiple Projects

The same external database may be connected to multiple projects.

Example:

    Project A
      |
      +--- PostgreSQL Database A
             selected tables:
             users
             organizations

    Project B
      |
      +--- PostgreSQL Database A
             selected tables:
             billing
             subscriptions

Each project has its own database source configuration.

The system must not assume that an external database belongs exclusively to one project.

---

# 19. Credential Deduplication

Do not initially attempt to globally deduplicate credentials across projects.

Project isolation is more important.

If two projects connect to the same external database, they should have independent source configurations.

Future versions may introduce a shared connection/credential model, but that is outside the initial scope.

---

# 20. Database Metadata Model

Normalize database metadata into a common structure.

Example:

    type DatabaseMetadata struct {
        DatabaseType string
        DatabaseName string
        Version      string
        Schemas      []SchemaMetadata
    }

Schema:

    type SchemaMetadata struct {
        Name   string
        Tables []TableMetadata
    }

Table:

    type TableMetadata struct {
        Schema      string
        Name        string
        Type        string
        Comment     string
        Columns     []ColumnMetadata
        PrimaryKey  []string
        ForeignKeys []ForeignKeyMetadata
        Indexes     []IndexMetadata
    }

Column:

    type ColumnMetadata struct {
        Name         string
        DataType     string
        Nullable     bool
        DefaultValue string
        Comment      string
        Position     int
    }

Foreign key:

    type ForeignKeyMetadata struct {
        Name              string
        Columns           []string
        ReferencedSchema  string
        ReferencedTable   string
        ReferencedColumns []string
    }

Database engines may not support every metadata field.

Missing metadata should be represented cleanly rather than causing the entire extraction to fail.

---

# 21. Schema-Only Ingestion

Schema-only ingestion is the default.

For example:

    Table: users

    Columns:
    - id: UUID PRIMARY KEY
    - email: VARCHAR NOT NULL UNIQUE
    - organization_id: UUID
    - created_at: TIMESTAMP

    Relationships:
    - users.organization_id -> organizations.id

This information becomes a knowledge document.

The system should generate human-readable normalized content suitable for embedding.

---

# 22. Row-Level Data Ingestion

Row-level ingestion must be explicitly opt-in.

Do not automatically index all rows.

The user should select which tables are allowed.

Example:

    Tables

    [ ] users
    [ ] sessions
    [x] products
    [x] plans

The user may explicitly enable:

    [x] Index selected table data

The UI must display a warning explaining that database records may contain sensitive information.

---

# 23. Column Selection and Exclusion

The ingestion configuration should support column-level exclusions.

Example:

    {
      "excluded_columns": {
        "users": [
          "password_hash",
          "reset_token",
          "api_key"
        ]
      }
    }

The application must never ingest excluded columns.

---

# 24. Sensitive Column Detection

The system should optionally warn when columns appear to contain sensitive information.

Examples:

- password
- password_hash
- passwd
- secret
- token
- access_token
- refresh_token
- api_key
- private_key
- secret_key
- credit_card
- card_number
- cvv
- ssn

Do not automatically block all columns with these names in the initial implementation unless explicitly required.

Instead:

1. Detect likely sensitive columns.
2. Display a warning.
3. Make exclusion easy.
4. Require explicit opt-in for row-level ingestion.

---

# 25. Extraction Modes

The source should support:

    schema
    schema_and_data

Default:

    schema

Future modes may include:

    selected_rows
    custom_query

Do not implement arbitrary custom SQL in the initial version.

---

# 26. Knowledge Document Representation

All database information must be converted into the platform's common knowledge-document model.

Example:

    type KnowledgeDocument struct {
        SourceID     string
        ExternalID   string
        Title        string
        Content      string
        SourceType   string
        SourceURL    string
        Metadata     map[string]any
        ContentHash  string
    }

Example metadata:

    {
      "database_type": "postgres",
      "schema": "public",
      "table": "users",
      "object_type": "table"
    }

The database connector should produce normalized knowledge documents or normalized extraction results that can be transformed into the application's existing knowledge document model.

Prefer reusing the application's existing knowledge model if one already exists.

---

# 27. Example Generated Document

For a table:

    public.users

Generate normalized content similar to:

    Database: application
    Schema: public
    Table: users

    Description:
    Stores application users.

    Columns:

    id
    Type: uuid
    Nullable: false
    Primary Key: yes

    email
    Type: varchar
    Nullable: false

    organization_id
    Type: uuid
    Nullable: true

    created_at
    Type: timestamp
    Nullable: false

    Relationships:

    users.organization_id references public.organizations.id

The exact formatting may evolve.

The content should prioritize retrieval quality and clarity.

---

# 28. Chunking

Database documents should pass through the same general chunking pipeline as other knowledge sources where appropriate.

However, table schemas should preferably remain logically grouped.

Avoid splitting the table name from its columns if doing so would significantly reduce retrieval quality.

The chunking strategy should be configurable through the existing ingestion architecture.

Do not create a completely separate embedding pipeline only for databases unless technically necessary.

---

# 29. Embeddings

Database knowledge documents must use the application's configured embedding provider.

The database connector must not call the embedding provider directly.

Required architecture:

    DatabaseConnector
           |
           v
    KnowledgeDocument
           |
           v
    IngestionPipeline
           |
           v
         Chunker
           |
           v
    EmbeddingService
           |
           v
        pgvector

---

# 30. Project Isolation

Every database knowledge chunk must be associated with the project that owns the source.

Conceptually:

    knowledge_chunks
    ----------------
    id
    project_id
    source_id
    document_id
    content
    embedding
    metadata

Vector retrieval must always filter by project_id before returning results.

A database source from Project A must never contribute knowledge to Project B.

This must be enforced server-side.

Do not rely on the frontend to enforce project isolation.

---

# 31. Security Boundary

The required flow is:

    Authenticated User
           |
           v
    Project Authorization
           |
           v
    Database Source Authorization
           |
           v
    Database Operation

Never allow a request containing only a source_id to trigger an external database operation without verifying that the source belongs to a project the user is authorized to access.

---

# 32. API Endpoints

The exact route prefix should follow the application's existing API conventions.

Recommended endpoints:

    POST   /api/projects/{projectID}/sources/database
    GET    /api/projects/{projectID}/sources/database
    GET    /api/projects/{projectID}/sources/database/{sourceID}
    PATCH  /api/projects/{projectID}/sources/database/{sourceID}
    DELETE /api/projects/{projectID}/sources/database/{sourceID}

    POST   /api/projects/{projectID}/sources/database/test
    POST   /api/projects/{projectID}/sources/database/{sourceID}/test

    GET    /api/projects/{projectID}/sources/database/{sourceID}/metadata

    POST   /api/projects/{projectID}/sources/database/{sourceID}/sync

    GET    /api/projects/{projectID}/sources/database/{sourceID}/status

Do not expose raw credentials through GET endpoints.

---

# 33. Create Database Source

Request:

    {
      "name": "Application Database",
      "database_type": "postgres",
      "connection_url": "postgresql://user:password@host:5432/app"
    }

The backend should:

1. Authenticate the user.
2. Authorize access to the project.
3. Validate the database type.
4. Parse the connection URL.
5. Validate required fields.
6. Test the connection.
7. Introspect basic metadata.
8. Encrypt credentials.
9. Store the source.
10. Return sanitized source metadata.
11. Allow the user to configure ingestion.
12. Start ingestion only through the explicit ingestion workflow.

Do not perform a full data ingestion synchronously.

---

# 34. Connection Test

The test endpoint should:

1. Resolve the requested connector.
2. Parse connection information.
3. Establish a connection.
4. Execute a safe metadata query.
5. Close the connection.
6. Return a sanitized result.

Success:

    {
      "success": true,
      "database_type": "postgres",
      "database_version": "16.x"
    }

Failure:

    {
      "success": false,
      "error_code": "DATABASE_CONNECTION_FAILED",
      "message": "Unable to connect to the database."
    }

Do not return raw driver errors containing credentials or infrastructure secrets.

---

# 35. Metadata Endpoint

After connection succeeds, users should be able to retrieve metadata.

Example:

    GET /api/projects/{projectID}/sources/database/{sourceID}/metadata

Response:

    {
      "database": "app",
      "schemas": [
        {
          "name": "public",
          "tables": [
            {
              "name": "users",
              "columns": [
                {
                  "name": "id",
                  "type": "uuid"
                }
              ]
            }
          ]
        }
      ]
    }

The metadata endpoint must use the stored encrypted credentials internally and must never return them.

---

# 36. Ingestion Configuration

The user should be able to configure:

- Schemas
- Tables
- Columns
- Ingestion mode
- Excluded columns

Example:

    {
      "mode": "schema",
      "schemas": [
        {
          "name": "public",
          "tables": [
            {
              "name": "users"
            },
            {
              "name": "organizations"
            }
          ]
        }
      ]
    }

The backend must validate that selected schemas/tables actually exist before ingestion.

---

# 37. Ingestion Job

Database ingestion must be asynchronous.

Required flow:

    POST /sync
           |
           v
    Create ingestion job
           |
           v
    Return 202 Accepted
           |
           v
    Background worker
           |
           v
    Connect to database
           |
           v
    Read metadata/data
           |
           v
    Normalize
           |
           v
    Chunk
           |
           v
    Generate embeddings
           |
           v
    Persist vectors
           |
           v
    Mark job complete

Do not block HTTP requests while performing a complete database ingestion.

---

# 38. Ingestion Status

Supported states:

    pending
    running
    completed
    failed
    cancelled

Example:

    {
      "status": "running",
      "documents_discovered": 125,
      "documents_processed": 83,
      "chunks_created": 420,
      "started_at": "...",
      "updated_at": "..."
    }

The exact progress model may be adjusted based on the existing job system.

---

# 39. Error Handling

Errors should be categorized.

Examples:

    DATABASE_CONNECTION_FAILED
    DATABASE_AUTHENTICATION_FAILED
    DATABASE_PERMISSION_DENIED
    DATABASE_TIMEOUT
    DATABASE_UNSUPPORTED
    DATABASE_METADATA_FAILED
    DATABASE_EXTRACTION_FAILED
    DATABASE_EMBEDDING_FAILED
    INGESTION_FAILED

Errors should be:

- Sanitized
- Structured
- Logged with safe metadata
- Visible to the user at an appropriate level

---

# 40. Timeouts

All external database operations must have context-aware timeouts.

Never allow an external database request to hang indefinitely.

Use:

    context.Context

throughout connector operations.

Connection, metadata, and extraction operations should have configurable limits.

---

# 41. Connection Pooling

The connector must not maintain uncontrolled connection pools.

For short ingestion jobs, a bounded pool may be used.

The application must:

- Close connections correctly.
- Respect external database limits.
- Avoid opening one connection per table.
- Avoid creating unbounded concurrent database queries.

---

# 42. Concurrency

Do not immediately parallelize all table extraction.

An external database may have limited resources.

The ingestion worker should have bounded concurrency.

Example configuration:

    DB_INGESTION_MAX_CONCURRENCY=4

The exact default can be selected during implementation.

---

# 43. Incremental Synchronization

The system should support incremental ingestion where practical.

Use canonical representations and content hashes.

For example:

    Table metadata
         |
         v
    Canonical representation
         |
         v
    SHA-256 hash

If the hash has not changed:

    skip re-embedding

If it changed:

    replace affected knowledge chunks

The implementation should avoid unnecessarily creating duplicate chunks.

---

# 44. Source Versioning

Each ingestion should have a logical version.

For schema ingestion, the version may be based on the canonical schema metadata hash.

For row-level data, the versioning strategy may differ.

The implementation should avoid creating duplicate knowledge when the underlying database content has not materially changed.

---

# 45. Deletion

When a database source is deleted:

1. Verify project authorization.
2. Stop or cancel active ingestion jobs.
3. Delete source-specific knowledge documents.
4. Delete source-specific chunks and embeddings.
5. Delete encrypted credentials.
6. Delete source configuration.
7. Preserve audit logs where applicable.

Do not delete knowledge belonging to other projects or sources.

---

# 46. Re-Synchronization

Users must be able to manually trigger synchronization.

Example:

    POST /api/projects/{projectID}/sources/database/{sourceID}/sync

The endpoint should create a background job rather than block until completion.

If an ingestion job is already running, the API should avoid creating duplicate concurrent jobs unless explicitly supported by the existing job architecture.

---

# 47. Web UI Requirements

The UI should allow:

    Project
      |
      +-- Knowledge Sources
            |
            +-- Add Source
                  |
                  +-- Database

Database setup flow:

    1. Select database type
    2. Enter connection URL
    3. Test connection
    4. Load database metadata
    5. Select schemas
    6. Select tables
    7. Configure ingestion
    8. Confirm
    9. Start ingestion
    10. Show ingestion status

The UI should use the application's API rather than directly connecting to external databases.

---

# 48. Database Source UI

Display:

- Name
- Database type
- Host
- Database name
- Selected schemas
- Selected tables
- Ingestion mode
- Status
- Last synchronized
- Last synchronization error, if any

Never display:

- Password
- Full connection URL
- API secrets
- Tokens
- Private keys

---

# 49. Connection URL Input

The UI should provide:

    Database Type:
    [ PostgreSQL ]

    Connection URL:
    [ postgresql://user:password@host:5432/database ]

    [ Test Connection ]

Connection URLs should be treated as sensitive input.

The UI should avoid persisting the raw URL in browser local storage.

Do not place connection URLs into analytics events.

---

# 50. UI Connection Test

The user should receive clear feedback.

Success:

    ✓ Connection successful

Failure:

    ✗ Connection failed

    Unable to authenticate with the database.
    Check the username, password, and permissions.

Avoid exposing raw driver errors to users.

---

# 51. UI Security Warning

When enabling row-level ingestion, show a warning similar to:

    Database records may contain sensitive or personal information.
    Only enable data ingestion for tables and columns that are safe
    to store in the knowledge base.

The warning must be visible before confirmation.

---

# 52. Database Permissions

Documentation should recommend creating a dedicated read-only database user.

Conceptually:

    Application
        |
        +--- RAG reader account
                 |
                 +--- SELECT

The application should not require administrative permissions.

Where practical, provide database-specific documentation for creating a read-only account.

---

# 53. Network Connectivity

The application may need to connect to databases that are:

- Local
- Docker-hosted
- LAN-hosted
- Cloud-hosted
- Behind a firewall

The initial implementation only needs to support databases reachable from the backend/worker runtime.

Do not attempt to build tunneling or VPN functionality.

Document common Docker networking issues.

For example, `localhost` from inside a container refers to the container itself, not necessarily the host machine.

---

# 54. SSRF and Network Security

Because users can provide arbitrary database connection URLs, this feature creates a potential server-side network access risk.

The implementation must explicitly consider SSRF.

At minimum:

- Validate connection schemes.
- Validate ports.
- Reject unsupported protocols.
- Do not allow arbitrary command execution through connection URLs.
- Never pass connection URLs into shell commands.
- Never interpolate connection URL components into shell commands.
- Sanitize driver options.
- Do not permit arbitrary driver/plugin loading through user input.

The deployment should provide a configurable network access policy.

Depending on deployment requirements, private IP ranges may need to be blocked by default or controlled through an explicit allowlist.

Document the chosen security policy.

---

# 55. SQL Injection Protection

Never construct SQL using untrusted schema/table names without safe identifier handling.

Do not build queries such as:

    query := "SELECT * FROM " + userProvidedTable

without validating and safely quoting identifiers.

Prefer:

- Database-specific identifier quoting.
- Validated identifiers.
- Parameterized values.
- Connector-specific safe query builders where appropriate.

Values must always be parameterized.

---

# 56. Database Driver Dependencies

Use mature, actively maintained Go database drivers.

The implementation agent must evaluate current stable drivers before implementation.

Do not write custom database protocol implementations.

Drivers must be licensed compatibly with the open-source project.

Record selected drivers and their licenses in project documentation.

---

# 57. ORM Boundary

The application's own PostgreSQL database must continue to be accessed through the selected ORM/data-access layer.

External database connectors should NOT be forced through the application's ORM.

Recommended architecture:

    Application Database
        |
        +--- ORM
        |
        +--- PostgreSQL + pgvector

    External Database
        |
        +--- DatabaseConnector
                |
                +--- database/sql or driver-specific API

External databases may have different schemas and drivers, so they should be handled through connector-specific data-access code.

---

# 58. Testing Strategy

The feature must include unit, integration, API, security, and end-to-end tests.

## 58.1 Unit Tests

Test:

- Connection URL parsing
- Database type validation
- Connector registry
- Metadata normalization
- Schema normalization
- Column normalization
- Sensitive-column detection
- Hash generation
- Configuration validation
- Authorization logic
- Error sanitization

## 58.2 Integration Tests

Use Docker containers where practical for:

- PostgreSQL
- MySQL
- MariaDB
- SQL Server

SQLite can run directly in tests.

Tests should verify:

- Connection
- Metadata discovery
- Schema extraction
- Table extraction
- Column extraction
- Primary keys
- Foreign keys
- Indexes
- Comments where supported
- Ingestion
- Incremental sync
- Deletion

CockroachDB integration tests should be included where CI resources permit.

## 58.3 API Tests

Test:

- Create source
- List source
- Get source
- Update source
- Delete source
- Test connection
- Retrieve metadata
- Start sync
- Retrieve sync status

## 58.4 End-to-End Tests

Test:

    Create project
        ↓
    Add database source
        ↓
    Test connection
        ↓
    Load metadata
        ↓
    Select tables
        ↓
    Start ingestion
        ↓
    Wait for completion
        ↓
    Ask RAG question
        ↓
    Verify database knowledge is retrieved
        ↓
    Verify citation/source metadata
        ↓
    Verify project isolation
        ↓
    Delete source
        ↓
    Verify vectors are removed

The complete workflow must succeed from a clean environment.

---

# 59. Project Isolation Tests

This is mandatory.

Create:

    Project A
    Project B

Connect the same external database to both projects with different table selections.

Example:

    Project A:
    users

    Project B:
    billing

Ask Project A questions about billing.

Expected:

    No billing knowledge returned.

Ask Project B about users.

Expected:

    No users knowledge returned.

The test must prove isolation at the backend/vector retrieval layer, not only at the UI layer.

---

# 60. Credential Security Tests

Tests must verify:

- Password is not returned from API.
- Connection URL is not returned.
- Password is not logged.
- Database credentials are encrypted at rest.
- Unauthorized users cannot access credentials.
- Deleting a source removes credentials.
- API errors do not leak credentials.
- Connection test errors do not leak credentials.
- Ingestion errors do not leak credentials.

---

# 61. API Authorization Tests

Verify scenarios such as:

    User A + Project A → allowed
    User A + Project B → denied
    User B + Project A → denied

assuming the users do not have appropriate project permissions.

Also test attempts to access a database source belonging to another project.

---

# 62. Observability

The feature should expose safe metrics such as:

    database_connection_attempts_total
    database_connection_failures_total
    database_ingestion_jobs_total
    database_ingestion_failures_total
    database_documents_processed_total
    database_chunks_created_total
    database_ingestion_duration_seconds

Do not include credentials in metrics labels.

Avoid high-cardinality labels such as raw connection URLs.

---

# 63. Audit Events

Where the main application supports auditing, database source operations should generate audit events.

Examples:

    database_source_created
    database_source_updated
    database_source_deleted
    database_source_connection_tested
    database_source_sync_started
    database_source_sync_completed
    database_source_sync_failed

Audit events must never contain secrets.

---

# 64. Rate Limiting

Database ingestion should be protected against accidental resource exhaustion.

Use bounded:

- Concurrent jobs
- Database connections
- Table extraction
- Embedding requests

The application should prevent a user from starting unlimited simultaneous ingestion jobs.

---

# 65. Retry Strategy

Retry transient errors such as:

- Temporary network failures
- Connection resets
- Temporary embedding provider failures

Do not blindly retry:

- Authentication failures
- Permission denied
- Invalid connection configuration
- Unsupported database type

Use exponential backoff where appropriate.

---

# 66. Data Lifecycle

The application should clearly distinguish:

    External Database
            |
            | read
            v
    Knowledge Documents
            |
            v
          Chunks
            |
            v
        Embeddings

The application does not become a mirror of the external database unless the user explicitly enables row-level ingestion.

---

# 67. Data Freshness

The source UI should show information such as:

    Last synchronized:
    September 8, 2026 14:30

    Status:
    Healthy

If synchronization fails:

    Status:
    Sync failed

    Last successful sync:
    September 8, 2026 14:30

Do not delete the previous successful knowledge index merely because a later sync failed.

---

# 68. Failure Semantics

A failed synchronization must not leave the project with a partially corrupted knowledge index.

Prefer a staged ingestion approach:

    Existing index
          |
          v
    New ingestion
          |
          v
       Validate
          |
          v
    Commit new version

If ingestion fails:

    keep previous successful version

The exact implementation may use staging tables, version markers, transactions, or another safe replacement strategy depending on the application's architecture.

---

# 69. Database Source Lifecycle

Recommended lifecycle:

    created
       |
       v
    connection_tested
       |
       v
    configured
       |
       v
    ingesting
       |
       v
    ready

Recoverable error state:

    failed

The source should remain available for retry after recoverable failures.

---

# 70. API Response Design

Use consistent API response structures.

Do not return internal database models directly.

Create explicit API DTOs.

Example:

    type DatabaseSourceResponse struct {
        ID           string     `json:"id"`
        Name         string     `json:"name"`
        DatabaseType string     `json:"database_type"`
        Host         string     `json:"host"`
        DatabaseName string     `json:"database_name"`
        Status       string     `json:"status"`
        LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
        CreatedAt    time.Time  `json:"created_at"`
        UpdatedAt    time.Time  `json:"updated_at"`
    }

Never include:

- password
- encrypted_password
- raw_connection_url
- encryption key
- secret configuration

---

# 71. Database Source Configuration

Store ingestion configuration separately from connection credentials where practical.

Example:

    {
      "mode": "schema",
      "schemas": [
        "public"
      ],
      "tables": [
        "users",
        "organizations"
      ],
      "excluded_columns": {
        "users": [
          "password_hash"
        ]
      }
    }

The configuration must be validated server-side.

---

# 72. Database Source Naming

Users should provide a human-readable name.

Examples:

- Production PostgreSQL
- Staging Database
- Billing Database
- Analytics DB
- Local Development DB

Names do not need to be globally unique.

They only need to be unique or clearly identifiable within the project if that matches the application's existing UX conventions.

---

# 73. Database Type Selection

The UI should provide:

    PostgreSQL
    MySQL
    MariaDB
    SQLite
    SQL Server
    CockroachDB

The backend must validate that the selected connector can parse the supplied connection configuration.

Do not rely only on the client-side database type selector.

---

# 74. Metadata Discovery Performance

Metadata discovery should be lightweight.

The initial metadata request should not read entire tables.

For schema-only ingestion, the connector should use system catalogs/information schemas or database-specific metadata APIs.

Do not execute:

    SELECT * FROM every table

during metadata discovery.

---

# 75. Row-Level Extraction Safety

When row-level ingestion is enabled:

- Process only explicitly selected tables.
- Process only explicitly allowed columns.
- Never read excluded columns.
- Use bounded batch sizes.
- Use parameterized queries.
- Avoid loading an entire large table into memory.
- Stream or paginate rows.
- Respect cancellation through context.Context.
- Stop safely when ingestion is cancelled.

The ingestion implementation must be memory-bounded.

---

# 76. Large Table Handling

Large tables must not be loaded entirely into memory.

Use batching.

Conceptually:

    Table
      |
      +--- Batch 1
      +--- Batch 2
      +--- Batch 3
      +--- ...
      +--- Batch N

Each batch should be normalized and processed incrementally.

The exact batch size should be configurable.

---

# 77. Database Comments

Where supported, ingest useful database comments/descriptions.

Examples:

- Table comments
- Column comments

Comments often contain valuable domain knowledge and should be included in the generated knowledge document.

---

# 78. Relationships

Foreign-key relationships should be included in schema knowledge.

Example:

    users.organization_id
        ->
    organizations.id

This is important for RAG because relationships often answer questions that individual table schemas cannot.

---

# 79. Index Information

Where supported, include relevant index metadata.

For example:

    users.email
        UNIQUE INDEX

Avoid embedding low-value implementation details if they negatively affect retrieval quality.

The connector should expose the metadata, while the knowledge normalization layer decides what is useful for RAG.

---

# 80. Views

Where supported, database views should be treated as separate database objects.

The implementation should distinguish:

    table
    view

Views may be included in schema ingestion.

Do not automatically execute arbitrary view queries during schema-only ingestion.

---

# 81. Materialized Views

Where supported, materialized views should be represented separately from ordinary tables/views.

Do not assume all databases expose materialized views using the same metadata APIs.

---

# 82. Stored Procedures and Functions

Do not ingest stored procedure/function bodies in the initial implementation unless the connector architecture can support them cleanly.

The initial focus is:

- Schemas
- Tables
- Columns
- Relationships
- Indexes
- Comments
- Views

Future support may add database routines.

---

# 83. Transactions

External database reads should use transactions only where necessary.

Do not hold long-running transactions during the entire ingestion process.

For large ingestion jobs, prefer short-lived operations or consistent snapshot mechanisms supported by the database where appropriate.

---

# 84. Cancellation

Ingestion must support cancellation.

If a user cancels an ingestion job:

1. Mark the job as cancelling/cancelled according to the existing job model.
2. Cancel the context.
3. Stop database extraction.
4. Stop embedding work where possible.
5. Clean up temporary/staging data.
6. Do not publish an incomplete index as the latest successful version.

---

# 85. Connection Cleanup

Every external database connection must be closed.

Tests should verify that:

- Connections are closed.
- Rows are closed.
- Statements are closed.
- Transactions are rolled back when appropriate.
- Context cancellation releases resources.

Avoid resource leaks during repeated synchronization.

---

# 86. Connector-Specific Behavior

Each connector may have database-specific behavior.

Do not force every database to implement unsupported metadata.

For example:

    PostgreSQL
      supports rich schema metadata

    SQLite
      has a simpler metadata model

    MySQL
      has information_schema

    SQL Server
      has sys catalog views

The common interface should expose normalized information while allowing connector-specific implementation details internally.

---

# 87. Driver Compatibility

Before selecting drivers, the implementation agent must verify:

- Current maintenance status
- Go version compatibility
- Database version compatibility
- License compatibility
- TLS support
- Context cancellation support
- Connection pooling behavior
- CI compatibility

Document the selected drivers.

---

# 88. Documentation Requirements

Add documentation covering:

    docs/database-sources.md
    docs/security/database-credentials.md
    docs/connectors/postgresql.md
    docs/connectors/mysql.md
    docs/connectors/sqlite.md
    docs/connectors/sql-server.md
    docs/connectors/cockroachdb.md

At minimum, explain:

- Supported databases
- Connection URL formats
- Required permissions
- Read-only account setup
- Docker networking
- Security considerations
- Row-level ingestion risks
- Sensitive column handling
- Troubleshooting

---

# 89. Architecture Decision Record

Create an ADR explaining the external database knowledge-source architecture.

Suggested title:

    ADR: External Database Knowledge Sources

Include:

## Context

Projects contain valuable database schema and optionally selected data that can improve RAG retrieval.

## Decision

Use a dedicated database connector abstraction with a connector registry.

## Consequences

Benefits:

- Database-specific logic is isolated.
- New database engines can be added independently.
- Core ingestion remains database-agnostic.
- Project-level configuration remains consistent.
- External database access remains read-only.

Tradeoffs:

- Each database engine requires connector maintenance.
- Database security becomes an important part of the system.
- Different databases expose different metadata capabilities.
- Row-level ingestion introduces additional privacy and security concerns.

---

# 90. OpenAPI

All database source endpoints must be documented in OpenAPI.

Document:

- Request schemas
- Response schemas
- Authentication
- Authorization
- Error responses
- Status codes
- Async ingestion behavior

Do not document credentials as response fields.

Mark connection URLs and credential fields as sensitive where OpenAPI supports appropriate annotations.

---

# 91. HTTP Status Codes

Recommended:

    201 Created
    400 Bad Request
    401 Unauthorized
    403 Forbidden
    404 Not Found
    409 Conflict
    422 Unprocessable Entity
    429 Too Many Requests
    500 Internal Server Error
    502 Bad Gateway
    503 Service Unavailable

For asynchronous ingestion:

    202 Accepted

---

# 92. Security Threat Model

Document at least the following threats.

## Credential theft

Mitigations:

- Encryption at rest
- Secure authentication/session handling
- No secret logging
- No secret API responses

## SSRF

Mitigations:

- Connection URL validation
- Network policies
- No shell execution
- Optional private-network restrictions
- Safe driver configuration

## Data exfiltration

Mitigations:

- Project isolation
- Read-only access
- Explicit table selection
- Explicit row-level opt-in

## Sensitive data ingestion

Mitigations:

- Schema-only default
- Column exclusions
- Sensitive-column warnings
- Explicit row-level ingestion

## Resource exhaustion

Mitigations:

- Bounded workers
- Timeouts
- Connection limits
- Batch processing
- Rate limits

## SQL injection

Mitigations:

- Safe identifier handling
- Parameterized values
- No arbitrary user-provided SQL
- No shell execution

---

# 93. Acceptance Criteria

The feature is complete when all of the following are true.

## Database Sources

- [ ] User can add a PostgreSQL database.
- [ ] User can add a MySQL database.
- [ ] User can add a MariaDB database.
- [ ] User can add a SQLite database.
- [ ] User can add a SQL Server database.
- [ ] User can add a CockroachDB database.
- [ ] Unsupported database types are rejected.

## Connection

- [ ] User can provide a connection URL.
- [ ] Connection can be tested.
- [ ] Credentials are encrypted.
- [ ] Credentials never appear in API responses.
- [ ] Credentials never appear in logs.
- [ ] Connection failures are sanitized.
- [ ] External database connections are closed correctly.

## Metadata

- [ ] Schemas can be discovered.
- [ ] Tables can be discovered.
- [ ] Columns can be discovered.
- [ ] Primary keys can be discovered where supported.
- [ ] Foreign keys can be discovered where supported.
- [ ] Indexes can be discovered where supported.
- [ ] Comments can be discovered where supported.
- [ ] Views can be discovered where supported.

## Ingestion

- [ ] Schema-only ingestion works.
- [ ] Schema-only ingestion is the default.
- [ ] Table selection works.
- [ ] Column exclusion works.
- [ ] Row-level ingestion requires explicit opt-in.
- [ ] Large tables are processed in bounded batches.
- [ ] Ingestion runs asynchronously.
- [ ] Ingestion status is visible.
- [ ] Failed ingestion does not destroy the previous successful index.
- [ ] Incremental ingestion avoids unnecessary re-embedding.
- [ ] Cancellation works safely.

## RAG

- [ ] Database knowledge is retrievable through RAG.
- [ ] Database chunks contain useful metadata.
- [ ] Citations can identify the database source.
- [ ] Citations can identify schema/table information where appropriate.
- [ ] Project-level retrieval filtering is enforced.
- [ ] Database knowledge can be combined with other project knowledge sources.

## Security

- [ ] Project authorization is enforced.
- [ ] Source authorization is enforced.
- [ ] Credentials are encrypted at rest.
- [ ] SSRF considerations are implemented/documented.
- [ ] External DB access is read-only.
- [ ] Sensitive fields are not accidentally ingested.
- [ ] Secrets do not leak through errors.
- [ ] Secrets do not leak through logs.
- [ ] Secrets do not leak through metrics.
- [ ] Secrets do not leak through audit events.
- [ ] Secrets do not leak through frontend storage.

## Testing

- [ ] Unit tests exist.
- [ ] Connector integration tests exist.
- [ ] Ingestion tests exist.
- [ ] API tests exist.
- [ ] Authorization tests exist.
- [ ] Project isolation tests exist.
- [ ] Credential security tests exist.
- [ ] End-to-end tests exist.

## Documentation

- [ ] OpenAPI documentation updated.
- [ ] Database source documentation exists.
- [ ] Security documentation exists.
- [ ] Connector documentation exists.
- [ ] ADR exists.
- [ ] Supported driver versions/licenses are documented.

---

# 94. Implementation Plan

The coding agent should implement the feature in phases.

## Phase 1 — Repository and Architecture Discovery

Tasks:

1. Inspect the existing repository.
2. Understand the existing project architecture.
3. Identify the existing source abstraction.
4. Identify the existing ingestion pipeline.
5. Identify the existing knowledge document model.
6. Identify the existing embedding service.
7. Identify the existing vector storage implementation.
8. Identify the existing project authorization system.
9. Identify the existing background job system.
10. Identify the existing API conventions.
11. Identify the existing frontend source-management UI.
12. Identify existing secret/encryption infrastructure.
13. Identify existing testing infrastructure.
14. Avoid creating duplicate abstractions.
15. Create or update the ADR.

Do not begin implementation until the existing architecture has been understood.

---

## Phase 2 — Connector Framework

Implement:

1. `DatabaseConnector`
2. `ConnectorRegistry`
3. Shared metadata models
4. Connection configuration
5. Extraction configuration
6. Normalized connector errors
7. Connector tests

The framework must be independent of the HTTP layer.

---

## Phase 3 — PostgreSQL Connector

Implement PostgreSQL first.

Support:

- Connection test
- Database metadata
- Schemas
- Tables
- Columns
- Primary keys
- Foreign keys
- Indexes
- Comments where available
- Views where practical
- Schema extraction

Add integration tests using Docker.

---

## Phase 4 — MySQL and MariaDB

Implement MySQL.

Reuse common logic where appropriate.

Add MariaDB compatibility.

Add integration tests.

---

## Phase 5 — SQLite

Implement SQLite.

Pay special attention to:

- File path handling
- Docker-mounted files
- Read-only access
- SQLite metadata tables
- File accessibility from the worker

Add integration tests.

---

## Phase 6 — SQL Server

Implement SQL Server connector.

Add integration tests where CI infrastructure permits.

---

## Phase 7 — CockroachDB

Implement CockroachDB using PostgreSQL compatibility where safe.

Add integration tests where practical.

---

## Phase 8 — Source Management

Implement:

- Create source
- Update source
- Delete source
- List sources
- Get source
- Test connection
- Metadata retrieval
- Source configuration
- Authorization
- Sanitized API DTOs

---

## Phase 9 — Ingestion

Implement:

- Ingestion job creation
- Background worker
- Metadata normalization
- Knowledge document generation
- Row-level extraction
- Batch processing
- Chunking
- Embeddings
- pgvector storage
- Progress tracking
- Failure handling
- Cancellation
- Versioning
- Incremental sync
- Safe replacement of previous successful index

---

## Phase 10 — Frontend

Implement:

- Add database source
- Database type selector
- Connection URL input
- Connection test
- Metadata browser
- Schema selector
- Table selector
- Column exclusion
- Row-level ingestion warning
- Ingestion configuration
- Sync button
- Sync status
- Last successful sync
- Error display
- Delete source

The frontend must use the backend API.

The frontend must never connect directly to external databases.

---

## Phase 11 — Security Hardening

Perform a dedicated security review focused on:

- SSRF
- Credential storage
- Credential leakage
- SQL injection
- Authorization
- Sensitive data ingestion
- Resource exhaustion
- Logging
- Metrics
- Audit events
- Browser storage
- Network access policy

---

## Phase 12 — End-to-End Verification

Run:

    Start application
        ↓
    Start PostgreSQL test database
        ↓
    Create project
        ↓
    Add database source
        ↓
    Test connection
        ↓
    Discover schema
        ↓
    Select tables
        ↓
    Start ingestion
        ↓
    Wait for completion
        ↓
    Ask RAG question
        ↓
    Verify database knowledge is retrieved
        ↓
    Verify citation
        ↓
    Verify project isolation
        ↓
    Modify external schema
        ↓
    Re-sync
        ↓
    Verify incremental update
        ↓
    Delete source
        ↓
    Verify vectors and knowledge documents are removed

The implementation is not complete until this workflow succeeds from a clean environment.

---

# 95. Agent Execution Rules

The coding agent must work iteratively.

Before implementation:

1. Inspect the repository.
2. Understand existing conventions.
3. Identify reusable components.
4. Identify integration points.
5. Create an implementation plan.
6. Break the work into small tasks.

During implementation:

1. Complete one task at a time.
2. Run relevant tests after each meaningful change.
3. Fix failures immediately.
4. Avoid speculative abstractions.
5. Do not duplicate existing functionality.
6. Keep changes focused.
7. Update documentation alongside implementation.
8. Keep security considerations visible throughout implementation.
9. Do not mark tasks complete without verification.
10. Maintain a task checklist.

After implementation:

1. Run formatting.
2. Run static analysis.
3. Run unit tests.
4. Run integration tests.
5. Run API tests.
6. Run security tests.
7. Run end-to-end tests.
8. Start the application from a clean environment.
9. Verify the complete database-source flow.
10. Review logs for credential leakage.
11. Review API responses for secret leakage.
12. Review frontend network requests for secret leakage.
13. Review project isolation.
14. Review deletion behavior.
15. Review failed ingestion behavior.
16. Review cancellation behavior.
17. Update documentation.
18. Fix all discovered issues.
19. Repeat verification until the feature works cleanly.

The agent must not stop after merely making the code compile.

---

# 96. Definition of Done

The database knowledge-source feature is considered production-ready when this complete workflow works reliably:

    Authenticated User
           |
           v
        Project
           |
           v
      Add Database
           |
           v
    Select DB Type
           |
           v
    Enter Connection URL
           |
           v
     Test Connection
           |
           v
    Discover Metadata
           |
           v
     Select Schemas
           |
           v
      Select Tables
           |
           v

Configure Ingestion
|
v
Start Sync
|
v
Background Worker
|
v
Extract
|
v
Normalize
|
v
Chunk
|
v
Embed
|
v
Store pgvector
|
v
Ask RAG Question
|
v
Project-Scoped Retrieval
|
v
AI Answer
|
v
Citations

The feature must be:

- Secure
- Project-isolated
- Read-only against external databases
- Observable
- Testable
- Documented
- Extensible
- Compatible with the existing RAG architecture

---

# 97. Future Enhancements

Do not implement these unless explicitly requested, but keep the architecture compatible with them:

- Scheduled database synchronization
- Webhooks/events where supported
- Change-data-capture ingestion
- Database schema diffing
- Schema visualization
- ER diagrams
- Row-level security awareness
- Automatic PII detection
- Automatic secret detection
- Custom SQL extraction
- Query-result knowledge sources
- MongoDB connector
- Snowflake connector
- BigQuery connector
- ClickHouse connector
- Neo4j connector
- Shared database credentials across projects
- Database health monitoring
- Database lineage
- Code-to-database relationship detection
- Automatic linking of GitHub code references to database tables
- Hybrid search combining semantic and keyword retrieval
- Database schema graph retrieval
- Cross-source dependency analysis
- Database migration awareness
- Database schema change notifications

---

# 98. Product Principle

The database feature should follow this principle:

> Connect once, understand safely, retrieve intelligently.

The purpose is not to turn the application into a database management tool.

The purpose is to make relevant database knowledge available to the project's RAG system while maintaining:

- Strict project isolation
- Credential security
- Read-only external access
- Explicit user control over indexed data
- Safe ingestion
- Extensible connector architecture
- High-quality retrieval
- Clear source attribution

The database connector layer should remain independent from the core RAG system so that adding a new database engine does not require changes to project management, retrieval, embeddings, or the frontend knowledge model.
