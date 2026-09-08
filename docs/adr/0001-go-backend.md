# ADR 0001: Go for Backend Services

## Status
Accepted

## Date
2026-09-08

## Context
ContextForge requires high concurrency, low latency, low memory overhead, and predictable execution performance for its API server and asynchronous ingestion workers. Ingestion workflows process thousands of source files, compute high-dimensional vector embeddings, communicate with external APIs, and execute local CLI tools.

We evaluated three potential backend ecosystems:
1. **Go (Golang)**: Native concurrency (goroutines/channels), single static binary deployment, low memory footprint, first-class standard library, excellent tooling.
2. **Python (FastAPI / Celery)**: Broad AI ecosystem, but higher runtime memory usage, GIL constraints, complex container dependency packaging, and slower raw execution speed.
3. **TypeScript / Node.js (NestJS / Fastify)**: Shared language with frontend, but higher memory footprint, single-threaded event loop bottleneck during heavy text chunking/processing, and weaker support for low-level OS child process pipelining.

## Decision
We chose **Go 1.23+** as the primary language for the ContextForge core API server and background workers.

Key architectural drivers:
- **Concurrency**: Goroutines allow efficient parallel ingestion, chunking, and vector embedding requests.
- **Resource Footprint**: Minimal memory footprint per connection, vital for self-hosted local installations.
- **Native Process Execution**: Standard `os/exec` provides robust, secure subprocess execution with timeout context handling for local coding agent CLIs (`opencode`, `claude`, `gemini`).
- **Production Operability**: Single static binary compilation simplifies multi-architecture containerization (`linux/amd64`, `linux/arm64`).

## Consequences

### Positive
- Extremely fast startup times (< 50ms) and low base memory consumption (< 30MB RSS).
- High throughput for SSE streaming without event loop blocking.
- Strict compile-time type safety preventing entire classes of runtime nil and type errors.
- Simple, predictable concurrency models for parallel workers and batch processing.

### Negative
- Less out-of-the-box RAG libraries compared to Python (e.g. LangChain / LlamaIndex). We build clean, lightweight internal interfaces instead of pulling heavy RAG frameworks.
- More verbose error handling patterns (`if err != nil`).

## Alternatives Considered
- **Python (FastAPI + Celery)**: Rejected due to memory overhead and GIL constraints during CPU-bound tokenization and text preprocessing.
- **Node.js (TypeScript)**: Rejected due to worker thread complexity and process isolation limitations.

## References
- SPEC.md: Section 1 (Executive Summary) & Section 4 (Architecture Overview)
- [Go Concurrency Patterns](https://go.dev/blog/pipelines)
