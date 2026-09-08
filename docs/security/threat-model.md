# ContextForge Security Threat Model (STRIDE)

This document provides a formal STRIDE threat analysis for ContextForge in accordance with [SPEC.md §94](file:///Users/jayanthsp/Desktop/Projects/ContextForge/SPEC.md).

---

## 1. Spoofing (Identity & Authenticity)

| Threat | Risk | Mitigation |
|---|---|---|
| Attacker intercepts or replays GitHub OAuth callback | High | State parameter verified via encrypted cookie; PKCE exchange required; redirect URIs strictly allowlisted. |
| Forged GitHub Webhooks trigger malicious indexing | High | `X-Hub-Signature-256` HMAC-SHA256 signature validation against `GITHUB_WEBHOOK_SECRET` before dispatching. |
| Session Token Hijacking | High | 256-bit cryptographically secure random session tokens; stored only as SHA-256 hash in database; delivered via `HttpOnly`, `Secure`, `SameSite=Lax` cookies. |

---

## 2. Tampering (Integrity & Injection)

| Threat | Risk | Mitigation |
|---|---|---|
| Indirect Prompt Injection via indexed repository files or issues | High | Untrusted context chunks are isolated in demarcated reference blocks; system prompt explicitly forbids treating reference text as instructions; adversarial test suite runs in CI. |
| Cross-tenant parameter tampering (`project_id` modification) | Critical | Every route under `/api/v1/projects/{id}/*` runs `RequireProjectAccess` verifying authenticated `user_id` ownership before handler execution. |
| Database SQL Injection (Internal) | Critical | All database access uses Ent ORM parameterization; raw pgvector SQL in `VectorRepository` strictly uses parameterized query placeholders (`$1`, `$2`). |
| SQL Injection in External Database Introspection | High | All dynamic schema, table, and column names are sanitized with engine-specific identifier quoting (`quotePgIdent`, `quoteMySQLIdent`, `quoteSQLiteIdent`, `quoteMSSQLIdent`), null-byte stripping (`\x00`), and parameterized catalog queries. |

---

## 3. Repudiation

| Threat | Risk | Mitigation |
|---|---|---|
| Untracked project deletion or unauthorized data alteration | Medium | Immutable audit logging in `audit_events` table recording user ID, action, project ID, timestamp, and client IP hash. |

---

## 4. Information Disclosure

| Threat | Risk | Mitigation |
|---|---|---|
| Cross-project vector retrieval leakage | Critical | Every vector query strictly includes `WHERE project_id = $project_id` in the database index scan. In-memory post-filtering is strictly forbidden. Non-negotiable cross-project isolation test runs in CI. |
| GitHub tokens or session keys exposed to frontend | Critical | Tokens and secrets are server-only. Frontend only receives session cookie. API error format scrubs internal stacks and tokens. |
| External Database credentials exposed in REST responses | Critical | Passwords and connection strings are encrypted at rest with AES-256-GCM. API responses return masked connection strings (`user:***@host:port/db`). Raw connection strings are never exposed after creation. |
| Sensitive columns (passwords, tokens, PII) exposed in row chunks | High | Sensitive column tokenization drops columns matching regex patterns (`password`, `token`, `secret`, `ssn`, etc.) before row batch normalization and chunk embedding. |
| Server-Side Request Forgery (SSRF) via URL ingestion or database connection | High | URL fetcher and database connectors validate target IPs against private RFC1918 subnets, loopback `127.0.0.0/8`, link-local `169.254.0.0/16` (cloud metadata), and IPv6 ULA/multicast. Pre-connection DNS lookups reject private IP targets. SQLite forbids sensitive system directories (`/root`, `/.ssh`, `/.env`). |
| Credentials leaked in application logs | High | Custom redaction filters and `SanitizeConnectionError` scrub authorization headers, cookies, API keys, and connection URL passwords from logs and error returns. |

---

## 5. Denial of Service (DoS)

| Threat | Risk | Mitigation |
|---|---|---|
| API flooding and expensive RAG search spam | High | Redis-backed sliding window rate limiter middleware returning HTTP 429 with `Retry-After`. |
| Ingestion queue starvation with massive repositories or databases | High | Per-source file limits (default max 10MB per file, 100MB per repo); bounded row sampling (`LIMIT 50`); prioritized queues (`critical`, `default`, `low`); distributed worker task timeouts. |
| Downstream LLM provider degradation freezing API threads | High | Context timeouts on all network calls; circuit breaker tripping after 5 consecutive provider failures. |

---

## 6. Elevation of Privilege

| Threat | Risk | Mitigation |
|---|---|---|
| Unauthorized access to another user's project by UUID guessing | Critical | All endpoints check user ownership in database. Unauthorized access returns `HTTP 404 Not Found` to eliminate project existence enumeration. |
| Cross-project access to external database sources | Critical | Dual-identifier lookups enforce `project_id = $project_id` filters; foreign project requests return RFC 7807 404 Not Found. |
| Ingestion background worker executing un-scoped operations | Critical | Asynq task payloads carry verified `project_id` and `user_id`; worker operations are strictly bounded by project foreign keys. |

