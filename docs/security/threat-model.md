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
| Database SQL Injection | Critical | All database access uses Ent ORM parameterization; raw pgvector SQL in `VectorRepository` strictly uses parameterized query placeholders (`$1`, `$2`). |

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
| Server-Side Request Forgery (SSRF) via URL ingestion | High | URL fetcher validates target IPs against private RFC1918 subnets, loopback `127.0.0.0/8`, link-local `169.254.0.0/16` (cloud metadata), and IPv6 unique local addresses. |
| Credentials leaked in application logs | High | Custom `log/slog` redaction handler scrubs Authorization headers, cookies, API keys, and token fields. |

---

## 5. Denial of Service (DoS)

| Threat | Risk | Mitigation |
|---|---|---|
| API flooding and expensive RAG search spam | High | Redis-backed sliding window rate limiter middleware returning HTTP 429 with `Retry-After`. |
| Ingestion queue starvation with massive repositories | High | Per-source file limits (default max 10MB per file, 100MB per repo); prioritized queues (`critical`, `default`, `low`); distributed worker task timeouts. |
| Downstream LLM provider degradation freezing API threads | High | Context timeouts on all network calls; circuit breaker tripping after 5 consecutive provider failures. |

---

## 6. Elevation of Privilege

| Threat | Risk | Mitigation |
|---|---|---|
| Unauthorized access to another user's project by UUID guessing | Critical | All endpoints check user ownership in database. Unauthorized access returns `HTTP 404 Not Found` to eliminate project existence enumeration. |
| Ingestion background worker executing un-scoped operations | Critical | Asynq task payloads carry verified `project_id` and `user_id`; worker operations are strictly bounded by project foreign keys. |
