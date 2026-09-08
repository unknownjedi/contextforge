# ContextForge Architecture Diagrams

## 1. System Topology

```mermaid
flowchart TD
    subgraph Client Tier
        UserBrowser[User Browser]
        NextApp[Next.js 14 Web Frontend\n:3000]
    end

    subgraph API Tier
        GoAPI[Go REST API Service\nGin Engine / :8080]
        AuthMiddleware[Auth & Project Tenant Isolation]
        RAGOrchestrator[RAG Orchestration Service]
    end

    subgraph Data & Queue Tier
        Postgres[(PostgreSQL 16\nRelational & Metadata)]
        PgVector[(pgvector\nHNSW Cosine Vector Index)]
        RedisQueue[(Redis 7\nAsynq Tasks & Rate Limiting)]
    end

    subgraph Asynchronous Worker Tier
        WorkerPool[Go Ingestion Workers\ncmd/worker]
        GHConnector[GitHub Ingestion Connector]
        DBConnector[Database Knowledge Connector\nPostgres/MySQL/SQLite/MSSQL/CRDB]
        Normalizer[Knowledge Normalizer\nDDL & Table Markdown Generator]
    end

    subgraph AI Provider Abstraction
        Embedder[Embedder Interface\nOllama / OpenAI / Gemini]
        Generator[Generator Interface\nBYOK or CLI: opencode/claude/gemini]
    end

    UserBrowser --> NextApp
    NextApp -->|REST API /api/v1| GoAPI
    GoAPI --> AuthMiddleware
    AuthMiddleware --> RAGOrchestrator

    RAGOrchestrator -->|VectorRepository| PgVector
    RAGOrchestrator -->|Query Embedding| Embedder
    RAGOrchestrator -->|Stream Response| Generator

    GoAPI -->|Ent ORM| Postgres
    GoAPI -->|Enqueue Jobs| RedisQueue

    RedisQueue -->|Dequeue Tasks| WorkerPool
    WorkerPool --> GHConnector
    WorkerPool --> DBConnector
    DBConnector --> Normalizer

    WorkerPool -->|Batch Embed| Embedder
    WorkerPool -->|Persist Chunks| PgVector
    WorkerPool -->|Update Job State| Postgres
```

## 2. Ingestion Pipeline Lifecycle

```mermaid
flowchart LR
    Source[Source Item\nGit File / DB Table / DB Rows] -->|Discover & Fetch| RawDoc[Raw Document]
    RawDoc -->|SHA-256| HashCheck{Content Changed?}
    HashCheck -- No --> Skip[Bypass Embedding\nZero AI Cost]
    HashCheck -- Yes --> Chunker[Code/Text Chunker]
    Chunker --> EmbedBatch[Batch Embedder]
    EmbedBatch --> StorageTx[Database Transaction]
    StorageTx -->|Replace Chunks| PgVector[(document_chunks\nproject_id indexed)]
```

## 3. Project-Scoped RAG Query Flow

```mermaid
sequenceDiagram
    autonumber
    actor User as User Browser
    participant API as Go REST API
    participant Middleware as Auth Middleware
    participant Embed as Embedder
    participant Repo as VectorRepository
    participant DB as PostgreSQL + pgvector
    participant LLM as Generator (BYOK or opencode CLI)

    User->>API: POST /api/v1/projects/{projectID}/chat/completions/stream
    API->>Middleware: Validate Session & Verify Project Access
    Middleware-->>API: Authorized (user owns project)
    API->>Embed: Embed(queryText)
    Embed-->>API: queryVector [float32]
    API->>Repo: SearchSimilar(projectID, queryVector, limit=5)
    Repo->>DB: SELECT ... WHERE project_id = $projectID ORDER BY embedding <=> $queryVector
    DB-->>Repo: Matched chunks (code files, DDL schemas, table rows)
    Repo-->>API: Retrieved chunks
    API->>LLM: GenerateStream(SystemPrompt, ContextChunks, Query)
    loop SSE Stream
        LLM-->>API: Token delta
        API-->>User: Server-Sent Event (message event with delta)
    end
    API-->>User: Structured Citation Event (source_id, file_path, lines, snippet, similarity)
```

## 4. External Database Knowledge Source Ingestion Flow

```mermaid
sequenceDiagram
    autonumber
    actor User as User / Admin
    participant API as ContextForge API
    participant DB as Internal PostgreSQL (Ent)
    participant Redis as Redis Asynq Queue
    participant Worker as ContextForge Worker
    participant ExtDB as External Database (Target)
    participant Norm as Knowledge Normalizer
    participant Embed as Vector Embedder
    participant Vec as pgvector Store

    User->>API: POST /projects/{id}/sources/database (URL, Engine, Config)
    API->>API: Validate SSRF & Test Connection
    API->>API: Encrypt URL with AES-256-GCM
    API->>DB: Persist DatabaseSource & Base Source
    API->>Redis: Enqueue database:sync task
    API-->>User: HTTP 201 Created (Masked credentials)

    Redis->>Worker: Dispatch database:sync job
    Worker->>Worker: Validate Target SSRF & Decrypt URL
    Worker->>ExtDB: Query Information Schema & Catalogs
    ExtDB-->>Worker: Tables, Columns, PKs, FKs, Indexes
    opt Mode is schema_and_data
        Worker->>Worker: Filter Sensitive Columns
        Worker->>ExtDB: Sample Table Rows (bounded SELECT LIMIT N)
        ExtDB-->>Worker: Row Records
    end
    Worker->>Norm: Generate DDL Markdown & Row Tables
    Norm-->>Worker: Structured Knowledge Documents
    Worker->>Worker: Calculate SHA-256 Content Hashes & Diff
    Worker->>Embed: Embed New/Modified Table Documents
    Embed-->>Worker: High-dimensional Vectors
    Worker->>Vec: Atomic Upsert into document_chunks
    Worker->>DB: Mark DatabaseSource status='ready'
```
