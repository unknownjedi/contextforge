# ADR 0006: API-First Design with Versioned REST, SSE Streaming, and OpenAPI 3.1

## Status
Accepted

## Date
2026-09-08

## Context
ContextForge is built to serve web browsers, desktop developer tools, coding agent plugins, and CI/CD pipelines. The backend interface must be predictable, discoverable, stable, and capable of real-time bidirectional or streaming interactions for LLM generation.

We evaluated communication protocols:
1. **REST with OpenAPI 3.1 + Server-Sent Events (SSE)**: Universal HTTP/JSON compatibility, standard client generator tooling, native streaming support, firewall-friendly.
2. **gRPC / Protocol Buffers**: High binary efficiency and strict typing, but requires gRPC-Web proxying for browser frontends, poor native cURL/API testing experience, and high overhead for external webhook ingestion.
3. **GraphQL**: Flexible client queries, but complex caching, security complexity (nested query denial-of-service), and non-standard streaming implementations.
4. **WebSockets**: Full duplex, but stateful connection management complicates load balancing, server restarts break connections, and SSE is sufficient for unilateral token streaming.

## Decision
We adopted an **API-First RESTful architecture with URL versioning (`/api/v1`)**, formal **OpenAPI 3.1 contract specification**, and **Server-Sent Events (SSE)** for AI token streaming.

Key architectural drivers:
- **Explicit Versioning**: All public endpoints are scoped under `/api/v1`. Breaking changes will increment the major path version (`/api/v2`).
- **Standardized Payloads**: All responses adhere to a consistent JSON envelope with error objects following RFC 7807 (Problem Details for HTTP APIs).
- **Server-Sent Events for RAG**: Text generation and citation delivery stream over HTTP/1.1 or HTTP/2 chunked transfer encoding (`text/event-stream`), avoiding WebSocket connection overhead.
- **OpenAPI 3.1 as Single Source of Truth**: The complete API surface is codified in `docs/api/openapi.yaml`, enabling automatic client SDK generation and contract testing.

## Consequences

### Positive
- Compatible with all programming languages, cURL, Postman, and browser fetch without custom client libraries.
- Easy to proxy, cache, inspect, and rate-limit using standard HTTP reverse proxies.
- Zero WebSocket connection state overhead on API servers; graceful reconnection via standard SSE `Last-Event-ID`.
- Automated frontend type-safety via TypeScript types generated directly from OpenAPI 3.1 schemas.

### Negative
- Streaming is unidirectional (server-to-client); user inputs must be sent via standard HTTP POST requests.
- SSE requires proxy buffering to be explicitly disabled in Nginx, Caddy, or Cloudflare.

## Alternatives Considered
- **WebSockets for All Chat**: Rejected because SSE is simpler, HTTP-native, reconnects automatically, and aligns with standard AI completion APIs (e.g. OpenAI / Anthropic stream protocol).
- **Unversioned Root URLs**: Rejected due to the impossibility of non-breaking future API migrations.

## References
- SPEC.md: Section 46 (API Versioning Strategy), Section 47 (OpenAPI Specification), Section 33 (Streaming Response Protocol)
- RFC 7807: Problem Details for HTTP APIs
