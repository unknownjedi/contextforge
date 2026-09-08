# ADR 0004: Asynchronous Ingestion Pipeline with Redis and Asynq

## Status
Accepted

## Date
2026-09-08

## Context
Ingesting software repositories, processing syntax trees, chunking source code, and generating vector embeddings are long-running, I/O-intensive, and computationally demanding tasks. Attempting to perform ingestion synchronously within an HTTP request lifecycle causes gateway timeouts, connection drops, server thread starvation, and unpredictable user latency.

We evaluated queue and background worker mechanisms in Go:
1. **Redis-backed Asynq**: Mature, Go-native distributed task queue with priority queues, exponential backoff retries, dead-letter queues (archived tasks), periodic tasks, and Redis-backed state visibility.
2. **RabbitMQ / AMQP**: Feature-rich message broker, but introduces additional infrastructure overhead, complex Erlang runtime, and lacks native Redis synergy.
3. **Kafka / Redpanda**: Enterprise streaming log, but heavily over-engineered for job queuing, high operational complexity, and lacking native delayed retry primitives.
4. **In-Memory Goroutines / Channels**: Zero infrastructure, but jobs are lost on process restart or deployment, with no cross-worker distribution or observability.

## Decision
We chose **Redis 7 with `hibiken/asynq`** for distributed asynchronous task processing.

Key architectural drivers:
- **Resilience & Crash Recovery**: Tasks in progress survive worker crashes and server restarts; Asynq automatically re-queues unacknowledged tasks.
- **Priority Queuing**: Multiple queues (`critical`, `default`, `low`) allow urgent single-file webhooks or user queries to supersede bulk repository re-indexing.
- **Backoff & Rate Limiting**: Native retry policies with exponential backoff and jitter protect external AI APIs and GitHub endpoints from cascading failures.
- **Idempotency**: All ingestion tasks include idempotency keys (`repo_id:commit_hash`) ensuring duplicate enqueue operations do not trigger duplicate processing.

## Consequences

### Positive
- Decoupled API responses: HTTP requests return `202 Accepted` immediately with a tracked `job_id`.
- Granular job lifecycle tracking (`pending`, `running`, `completed`, `failed`, `retrying`).
- Horizontally scalable workers independently deployable from API servers.
- Built-in Redis infrastructure reuse (caching, rate limiting, and task broker on single service).

### Negative
- Requires Redis as a required runtime dependency.
- Asynq payload data must be serialized as JSON/binary; large file blobs must be passed via database IDs or disk paths rather than giant queue payloads.

## Alternatives Considered
- **In-Memory Worker Pools**: Rejected due to risk of job loss during container restarts or deployments.
- **PostgreSQL-based Queue (e.g. `pg_boss` / `river`)**: Evaluated, but Redis-backed Asynq offers superior queue throughput, sub-millisecond polling latency, and reduces write amplification on the primary relational database.

## References
- SPEC.md: Section 17 (Asynchronous Ingestion Engine) & Section 18 (Queue Architecture)
- [Asynq Documentation](https://github.com/hibiken/asynq)
